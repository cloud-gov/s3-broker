package broker

import (
	"encoding/json"
	"errors"
	"fmt"
//	"strings"
	"time"

	"github.com/frodenas/brokerapi"
	"github.com/mitchellh/mapstructure"
	"github.com/pivotal-golang/lager"

	"github.com/apefactory/s3-broker/awsiam"
	"github.com/apefactory/s3-broker/awss3"
)

const instanceIDLogKey = "instance-id"
const bindingIDLogKey = "binding-id"
const detailsLogKey = "details"
const acceptsIncompleteLogKey = "acceptsIncomplete"

type S3Broker struct {
	bucketPrefix                 string
	allowUserProvisionParameters bool
	allowUserUpdateParameters    bool
	allowUserBindParameters      bool
	catalog                      Catalog
	bucket                       awss3.Bucket
	user                         awsiam.User
	logger                       lager.Logger
}

func New(
	config Config,
	bucket awss3.Bucket,
	user   awsiam.User,
	logger lager.Logger,
) *S3Broker {
	return &S3Broker{
		bucketPrefix:                 config.BucketPrefix,
		allowUserProvisionParameters: config.AllowUserProvisionParameters,
		allowUserUpdateParameters:    config.AllowUserUpdateParameters,
		catalog:                      config.Catalog,
		bucket:                 			bucket,
		logger:                       logger.Session("broker"),
	}
}

func (b *S3Broker) Services() brokerapi.CatalogResponse {
	catalogResponse := brokerapi.CatalogResponse{}

	brokerCatalog, err := json.Marshal(b.catalog)
	if err != nil {
		b.logger.Error("marshal-error", err)
		return catalogResponse
	}

	apiCatalog := brokerapi.Catalog{}
	if err = json.Unmarshal(brokerCatalog, &apiCatalog); err != nil {
		b.logger.Error("unmarshal-error", err)
		return catalogResponse
	}

	catalogResponse.Services = apiCatalog.Services

	return catalogResponse
}

func (b *S3Broker) Provision(instanceID string, details brokerapi.ProvisionDetails, acceptsIncomplete bool) (brokerapi.ProvisioningResponse, bool, error) {
	b.logger.Debug("provision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: acceptsIncomplete,
	})

	provisioningResponse := brokerapi.ProvisioningResponse{}
	if !acceptsIncomplete {
		return provisioningResponse, false, brokerapi.ErrAsyncRequired
	}

	provisionParameters := ProvisionParameters{}
	if b.allowUserProvisionParameters {
		if err := mapstructure.Decode(details.Parameters, &provisionParameters); err != nil {
			return provisioningResponse, false, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return provisioningResponse, false, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	var err error
	instance := b.createBucket(instanceID, servicePlan, provisionParameters, details)
	if _, err = b.bucket.Create(b.bucketName(instanceID), *instance); err != nil {
		return provisioningResponse, false, err
	}

	return provisioningResponse, false, nil
}

func (b *S3Broker) Update(instanceID string, details brokerapi.UpdateDetails, acceptsIncomplete bool) (bool, error) {
	b.logger.Debug("update", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: acceptsIncomplete,
	})

	if !acceptsIncomplete {
		return false, brokerapi.ErrAsyncRequired
	}

	updateParameters := UpdateParameters{}
	if b.allowUserUpdateParameters {
		if err := mapstructure.Decode(details.Parameters, &updateParameters); err != nil {
			return false, err
		}
	}

	service, ok := b.catalog.FindService(details.ServiceID)
	if !ok {
		return false, fmt.Errorf("Service '%s' not found", details.ServiceID)
	}

	if !service.PlanUpdateable {
		return false, brokerapi.ErrInstanceNotUpdateable
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return false, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	instance := b.modifyBucket(instanceID, servicePlan, updateParameters, details)
	if err := b.bucket.Modify(b.bucketName(instanceID), *instance); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return false, brokerapi.ErrInstanceDoesNotExist
		}
		return false, err
	}

	return true, nil
}

func (b *S3Broker) Deprovision(instanceID string, details brokerapi.DeprovisionDetails, acceptsIncomplete bool) (bool, error) {
	b.logger.Debug("deprovision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: acceptsIncomplete,
	})

	if !acceptsIncomplete {
		return false, brokerapi.ErrAsyncRequired
	}

	if err := b.bucket.Delete(b.bucketName(instanceID)); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return false, brokerapi.ErrInstanceDoesNotExist
		}
		return false, err
	}

	return true, nil
}

func (b *S3Broker) Bind(instanceID, bindingID string, details brokerapi.BindDetails) (brokerapi.BindingResponse, error) {
	b.logger.Debug("bind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	bindingResponse := brokerapi.BindingResponse{}

	service, ok := b.catalog.FindService(details.ServiceID)
	if !ok {
		return bindingResponse, fmt.Errorf("Service '%s' not found", details.ServiceID)
	}

	if !service.Bindable {
		return bindingResponse, brokerapi.ErrInstanceNotBindable
	}

	var accessKeyID, secretAccessKey string
	var policyARN string
	var err error

	if !service.Bindable {
		return bindingResponse, brokerapi.ErrInstanceNotBindable
	}

  bucketDetails, err := b.bucket.Describe(b.bucketName(instanceID))
	if err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return bindingResponse, brokerapi.ErrInstanceDoesNotExist
		}
		return bindingResponse, err
	}

	if _, err = b.user.Create(b.userName(bindingID)); err != nil {
		return bindingResponse, err
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
		return bindingResponse, err
	}

	policyARN, err = b.user.CreatePolicy(b.policyName(bindingID), "Allow", "s3:*", bucketDetails.ARN)
	if err != nil {
		return bindingResponse, err
	}

	if err = b.user.AttachUserPolicy(b.userName(bindingID), policyARN); err != nil {
		return bindingResponse, err
	}

	bindingResponse.Credentials = &brokerapi.CredentialsHash{
		Username: accessKeyID,
		Password: secretAccessKey,
		Name:     bucketDetails.BucketName,
	}

	return bindingResponse, nil

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

	userPolicies, err := b.user.ListAttachedUserPolicies(b.userName(bindingID))
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

func (b *S3Broker) LastOperation(instanceID string) (brokerapi.LastOperationResponse, error) {
	b.logger.Debug("last-operation", lager.Data{
		instanceIDLogKey: instanceID,
	})

	return brokerapi.LastOperationResponse{}, errors.New("This broker does not support LastOperation")
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
	return bucketDetails
}

func (b *S3Broker) modifyBucket(instanceID string, servicePlan ServicePlan, updateParameters UpdateParameters, details brokerapi.UpdateDetails) *awss3.BucketDetails {
	bucketDetails := b.bucketFromPlan(servicePlan)
	bucketDetails.Tags = b.bucketTags("Updated", details.ServiceID, details.PlanID, "", "")
	return bucketDetails
}

func (b *S3Broker) bucketFromPlan(servicePlan ServicePlan) *awss3.BucketDetails {
	bucketDetails := &awss3.BucketDetails{
	}
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
