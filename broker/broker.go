package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/mitchellh/mapstructure"
	"github.com/pivotal-cf/brokerapi"

	"github.com/cloudfoundry-community/s3-broker/awsiam"
	"github.com/cloudfoundry-community/s3-broker/awss3"
)

const instanceIDLogKey = "instance-id"
const bindingIDLogKey = "binding-id"
const detailsLogKey = "details"
const acceptsIncompleteLogKey = "acceptsIncomplete"

type S3Broker struct {
	iamPath                      string
	bucketPrefix                 string
	awsPartition                 string
	allowUserProvisionParameters bool
	allowUserUpdateParameters    bool
	allowUserBindParameters      bool
	catalog                      Catalog
	bucket                       awss3.Bucket
	user                         awsiam.User
	logger                       lager.Logger
}

type CatalogExternal struct {
	Services []brokerapi.Service `json:"services"`
}

func New(
	config Config,
	bucket awss3.Bucket,
	user awsiam.User,
	logger lager.Logger,
) *S3Broker {
	return &S3Broker{
		iamPath:                      config.IamPath,
		bucketPrefix:                 config.BucketPrefix,
		awsPartition:                 config.AwsPartition,
		allowUserProvisionParameters: config.AllowUserProvisionParameters,
		allowUserUpdateParameters:    config.AllowUserUpdateParameters,
		catalog:                      config.Catalog,
		bucket:                       bucket,
		user:                         user,
		logger:                       logger.Session("broker"),
	}
}

func (b *S3Broker) Services() []brokerapi.Service {
	brokerCatalog, err := json.Marshal(b.catalog)
	if err != nil {
		b.logger.Error("marshal-error", err)
		return []brokerapi.Service{}
	}

	apiCatalog := CatalogExternal{}
	if err = json.Unmarshal(brokerCatalog, &apiCatalog); err != nil {
		b.logger.Error("unmarshal-error", err)
		return []brokerapi.Service{}
	}

	return apiCatalog.Services
}

func (b *S3Broker) Provision(
	instanceID string,
	details brokerapi.ProvisionDetails,
	asyncAllowed bool,
) (brokerapi.ProvisionedServiceSpec, error) {
	b.logger.Debug("provision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	provisionParameters := ProvisionParameters{}
	if b.allowUserProvisionParameters {
		if err := mapstructure.Decode(details.RawParameters, &provisionParameters); err != nil {
			return brokerapi.ProvisionedServiceSpec{}, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return brokerapi.ProvisionedServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	var err error
	instance := b.createBucket(instanceID, servicePlan, provisionParameters, details)
	if _, err = b.bucket.Create(b.bucketName(instanceID), *instance); err != nil {
		return brokerapi.ProvisionedServiceSpec{}, err
	}

	return brokerapi.ProvisionedServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) Update(
	instanceID string,
	details brokerapi.UpdateDetails,
	asyncAllowed bool,
) (brokerapi.UpdateServiceSpec, error) {
	b.logger.Debug("update", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	updateParameters := UpdateParameters{}
	if b.allowUserUpdateParameters {
		if err := mapstructure.Decode(details.Parameters, &updateParameters); err != nil {
			return brokerapi.UpdateServiceSpec{}, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return brokerapi.UpdateServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	instance := b.modifyBucket(instanceID, servicePlan, updateParameters, details)
	if err := b.bucket.Modify(b.bucketName(instanceID), *instance); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return brokerapi.UpdateServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
		}
		return brokerapi.UpdateServiceSpec{}, err
	}

	return brokerapi.UpdateServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) Deprovision(
	instanceID string,
	details brokerapi.DeprovisionDetails,
	asyncAllowed bool,
) (brokerapi.DeprovisionServiceSpec, error) {
	b.logger.Debug("deprovision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	if err := b.bucket.Delete(b.bucketName(instanceID)); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
		}
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	return brokerapi.DeprovisionServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) Bind(instanceID, bindingID string, details brokerapi.BindDetails) (brokerapi.Binding, error) {
	b.logger.Debug("bind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	binding := brokerapi.Binding{}

	var accessKeyID, secretAccessKey string
	var policyARN string
	var err error

	bucketDetails, err := b.bucket.Describe(b.bucketName(instanceID), b.awsPartition)
	if err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return binding, brokerapi.ErrInstanceDoesNotExist
		}
		return binding, err
	}

	if _, err = b.user.Create(b.userName(bindingID), b.iamPath); err != nil {
		return binding, err
	}
	defer func() {
		if err != nil {
			if policyARN != "" {
				b.user.DeletePolicy(policyARN)
			}
			if accessKeyID != "" {
				b.user.DeleteAccessKey(b.userName(bindingID), accessKeyID)
			}
			b.user.Delete(b.userName(bindingID))
		}
	}()

	accessKeyID, secretAccessKey, err = b.user.CreateAccessKey(b.userName(bindingID))
	if err != nil {
		return binding, err
	}

	policyARN, err = b.user.CreatePolicy(b.policyName(bindingID), b.iamPath, "Allow", "s3:*", bucketDetails.ARN)
	if err != nil {
		return binding, err
	}

	if err = b.user.AttachUserPolicy(b.userName(bindingID), policyARN); err != nil {
		return binding, err
	}

	binding.Credentials = map[string]string{
		"username": accessKeyID,
		"password": secretAccessKey,
		"region":   bucketDetails.Region,
		"name":     bucketDetails.BucketName,
	}

	return binding, nil
}

func (b *S3Broker) Unbind(instanceID, bindingID string, details brokerapi.UnbindDetails) error {
	b.logger.Debug("unbind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	accessKeys, err := b.user.ListAccessKeys(b.userName(bindingID))
	if err != nil {
		return err
	}

	for _, accessKey := range accessKeys {
		if err := b.user.DeleteAccessKey(b.userName(bindingID), accessKey); err != nil {
			return err
		}
	}

	userPolicies, err := b.user.ListAttachedUserPolicies(b.userName(bindingID), b.iamPath)
	if err != nil {
		return err
	}

	for _, userPolicy := range userPolicies {
		if err := b.user.DetachUserPolicy(b.userName(bindingID), userPolicy); err != nil {
			return err
		}

		if err := b.user.DeletePolicy(userPolicy); err != nil {
			return err
		}
	}

	if err := b.user.Delete(b.userName(bindingID)); err != nil {
		return err
	}

	return nil
}

func (b *S3Broker) LastOperation(instanceID, operationData string) (brokerapi.LastOperation, error) {
	b.logger.Debug("last-operation", lager.Data{
		instanceIDLogKey: instanceID,
	})

	return brokerapi.LastOperation{}, errors.New("This broker does not support LastOperation")
}

func (b *S3Broker) bucketName(instanceID string) string {
	return fmt.Sprintf("%s-%s", b.bucketPrefix, instanceID)
}

func (b *S3Broker) userName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.bucketPrefix, bindingID)
}

func (b *S3Broker) policyName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.bucketPrefix, bindingID)
}

func (b *S3Broker) createBucket(instanceID string, servicePlan ServicePlan, provisionParameters ProvisionParameters, details brokerapi.ProvisionDetails) *awss3.BucketDetails {
	bucketDetails := b.bucketFromPlan(servicePlan)
	bucketDetails.Tags = b.bucketTags("Created", details.ServiceID, details.PlanID, details.OrganizationGUID, details.SpaceGUID)
	bucketDetails.Policy = string(servicePlan.S3Properties.Policy)
	bucketDetails.AwsPartition = b.awsPartition
	return bucketDetails
}

func (b *S3Broker) modifyBucket(instanceID string, servicePlan ServicePlan, updateParameters UpdateParameters, details brokerapi.UpdateDetails) *awss3.BucketDetails {
	bucketDetails := b.bucketFromPlan(servicePlan)
	bucketDetails.Tags = b.bucketTags("Updated", details.ServiceID, details.PlanID, "", "")
	return bucketDetails
}

func (b *S3Broker) bucketFromPlan(servicePlan ServicePlan) *awss3.BucketDetails {
	bucketDetails := &awss3.BucketDetails{}
	return bucketDetails
}

func (b *S3Broker) bucketTags(action, serviceID, planID, organizationID, spaceID string) map[string]string {
	tags := make(map[string]string)

	tags["Owner"] = "Cloud Foundry"

	tags[action+" by"] = "AWS S3 Service Broker"

	tags[action+" at"] = time.Now().Format(time.RFC822Z)

	if serviceID != "" {
		tags["Service ID"] = serviceID
	}

	if planID != "" {
		tags["Plan ID"] = planID
	}

	if organizationID != "" {
		tags["Organization ID"] = organizationID
	}

	if spaceID != "" {
		tags["Space ID"] = spaceID
	}
	return tags
}
