package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi"

	"github.com/cloudfoundry-community/s3-broker/awsiam"
	"github.com/cloudfoundry-community/s3-broker/awss3"
)

const instanceIDLogKey = "instance-id"
const bindingIDLogKey = "binding-id"
const detailsLogKey = "details"
const acceptsIncompleteLogKey = "acceptsIncomplete"

var (
	ErrNoClientConfigured = errors.New("This broker is not configured to support binding to additional instances. Contact your Cloud Foundry operator for details.")
)

type S3Broker struct {
	iamPath                      string
	userPrefix                   string
	policyPrefix                 string
	bucketPrefix                 string
	awsPartition                 string
	allowUserProvisionParameters bool
	allowUserUpdateParameters    bool
	allowUserBindParameters      bool
	catalog                      Catalog
	bucket                       awss3.Bucket
	user                         awsiam.User
	cfClient                     *cfclient.Client
	logger                       lager.Logger
}

type CatalogExternal struct {
	Services []brokerapi.Service `json:"services"`
}

type Credentials struct {
	URI               string   `json:"uri"`
	AccessKeyID       string   `json:"access_key_id"`
	SecretAccessKey   string   `json:"secret_access_key"`
	Region            string   `json:"region"`
	Bucket            string   `json:"bucket"`
	AdditionalBuckets []string `json:"additional_buckets"`
}

func New(
	config Config,
	bucket awss3.Bucket,
	user awsiam.User,
	cfClient *cfclient.Client,
	logger lager.Logger,
) *S3Broker {
	return &S3Broker{
		iamPath:                      config.IamPath,
		userPrefix:                   config.UserPrefix,
		policyPrefix:                 config.PolicyPrefix,
		bucketPrefix:                 config.BucketPrefix,
		awsPartition:                 config.AwsPartition,
		allowUserProvisionParameters: config.AllowUserProvisionParameters,
		allowUserUpdateParameters:    config.AllowUserUpdateParameters,
		catalog:                      config.Catalog,
		bucket:                       bucket,
		user:                         user,
		cfClient:                     cfClient,
		logger:                       logger.Session("broker"),
	}
}

func (b *S3Broker) Services(context context.Context) ([]brokerapi.Service, error) {
	brokerCatalog, err := json.Marshal(b.catalog)
	if err != nil {
		b.logger.Error("marshal-error", err)
		return []brokerapi.Service{}, err
	}

	apiCatalog := CatalogExternal{}
	if err = json.Unmarshal(brokerCatalog, &apiCatalog); err != nil {
		b.logger.Error("unmarshal-error", err)
		return []brokerapi.Service{}, err
	}

	return apiCatalog.Services, nil
}

func (b *S3Broker) Provision(
	context context.Context,
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
	if b.allowUserProvisionParameters && len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &provisionParameters); err != nil {
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
	context context.Context,
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
	if b.allowUserUpdateParameters && len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &updateParameters); err != nil {
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
	context context.Context,
	instanceID string,
	details brokerapi.DeprovisionDetails,
	asyncAllowed bool,
) (brokerapi.DeprovisionServiceSpec, error) {
	b.logger.Debug("deprovision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return brokerapi.DeprovisionServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}
	if err := b.bucket.Delete(b.bucketName(instanceID), servicePlan.Durable); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return brokerapi.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
		}
		return brokerapi.DeprovisionServiceSpec{}, err
	}

	return brokerapi.DeprovisionServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) GetBucketURI(credentials Credentials) string {
	var endpoint string
	if credentials.Region == "us-east-1" {
		endpoint = "s3.amazonaws.com"
	} else {
		endpoint = "s3-" + credentials.Region + ".amazonaws.com"
	}
	return fmt.Sprintf("s3://%s:%s@%s/%s",
		url.QueryEscape(credentials.AccessKeyID), url.QueryEscape(credentials.SecretAccessKey),
		endpoint, credentials.Bucket)
}

func (b *S3Broker) getBucketNames(instanceNames []string, instanceGUID string, planIDs []string) ([]string, error) {
	var bucketNames []string

	instance, err := b.cfClient.ServiceInstanceByGuid(instanceGUID)
	if err != nil {
		return nil, err
	}

	planQuery := url.Values{}
	planQuery.Add("q", fmt.Sprintf("unique_id IN %s", strings.Join(planIDs, ",")))
	plans, err := b.cfClient.ListServicePlansByQuery(planQuery)
	if err != nil {
		return nil, err
	}

	var planGUIDs []string
	for _, plan := range plans {
		planGUIDs = append(planGUIDs, plan.Guid)
	}

	query := url.Values{}
	query.Add("q", fmt.Sprintf("space_guid:%s", instance.SpaceGuid))
	query.Add("q", fmt.Sprintf("service_plan_guid IN %s", strings.Join(planGUIDs, ",")))
	instances, err := b.cfClient.ListServiceInstancesByQuery(query)
	if err != nil {
		return nil, err
	}

	instanceGUIDs := make(map[string]string, len(instanceNames))
	for _, instance := range instances {
		instanceGUIDs[instance.Name] = instance.Guid
	}

	for _, instanceName := range instanceNames {
		instanceGUID, ok := instanceGUIDs[instanceName]
		if !ok {
			return nil, fmt.Errorf("Service instance %s not found", instanceName)
		}
		bucketNames = append(bucketNames, b.bucketName(instanceGUID))
	}

	return bucketNames, nil
}

func (b *S3Broker) Bind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.BindDetails,
) (brokerapi.Binding, error) {
	b.logger.Debug("bind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	binding := brokerapi.Binding{}

	var accessKeyID, secretAccessKey string
	var policyARN string
	var err error

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return binding, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	bindParameters := BindParameters{}
	if len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &bindParameters); err != nil {
			return binding, err
		}
	}

	bucketNames := []string{b.bucketName(instanceID)}
	if len(bindParameters.AdditionalInstances) > 0 {
		if b.cfClient == nil {
			return binding, ErrNoClientConfigured
		}
		var planIDs []string
		for _, plan := range b.catalog.ListServicePlans() {
			planIDs = append(planIDs, plan.ID)
		}
		additionalNames, err := b.getBucketNames(bindParameters.AdditionalInstances, instanceID, planIDs)
		if err != nil {
			return binding, err
		}
		bucketNames = append(bucketNames, additionalNames...)
	}

	credentials := Credentials{AdditionalBuckets: []string{}}
	bucketARNs := make([]string, len(bucketNames))
	detailc, errc := make(chan awss3.BucketDetails), make(chan error)
	for _, bucketName := range bucketNames {
		go func(bucketName string) {
			bucketDetails, err := b.bucket.Describe(bucketName, b.awsPartition)
			if err != nil {
				if err == awss3.ErrBucketDoesNotExist {
					errc <- brokerapi.ErrInstanceDoesNotExist
				}
				errc <- err
			} else {
				detailc <- bucketDetails
			}
		}(bucketName)
	}
	for idx := range bucketNames {
		select {
		case bucketDetails := <-detailc:
			bucketARNs[idx] = bucketDetails.ARN
			if bucketDetails.BucketName == b.bucketName(instanceID) {
				credentials.Bucket = bucketDetails.BucketName
				credentials.Region = bucketDetails.Region
			} else {
				credentials.AdditionalBuckets = append(credentials.AdditionalBuckets, bucketDetails.BucketName)
			}
		case err := <-errc:
			return binding, err
		}
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

	policyARN, err = b.user.CreatePolicy(b.policyName(bindingID), b.iamPath, string(servicePlan.S3Properties.IamPolicy), bucketARNs)
	if err != nil {
		return binding, err
	}

	if err = b.user.AttachUserPolicy(b.userName(bindingID), policyARN); err != nil {
		return binding, err
	}

	credentials.AccessKeyID = accessKeyID
	credentials.SecretAccessKey = secretAccessKey
	credentials.URI = b.GetBucketURI(credentials)

	binding.Credentials = credentials

	return binding, nil
}

func (b *S3Broker) Unbind(
	context context.Context,
	instanceID, bindingID string,
	details brokerapi.UnbindDetails,
) error {
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

func (b *S3Broker) LastOperation(
	context context.Context,
	instanceID, operationData string,
) (brokerapi.LastOperation, error) {
	b.logger.Debug("last-operation", lager.Data{
		instanceIDLogKey: instanceID,
	})

	return brokerapi.LastOperation{}, errors.New("This broker does not support LastOperation")
}

func (b *S3Broker) bucketName(instanceID string) string {
	return fmt.Sprintf("%s-%s", b.bucketPrefix, instanceID)
}

func (b *S3Broker) userName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.userPrefix, bindingID)
}

func (b *S3Broker) policyName(bindingID string) string {
	return fmt.Sprintf("%s-%s", b.policyPrefix, bindingID)
}

func (b *S3Broker) createBucket(instanceID string, servicePlan ServicePlan, provisionParameters ProvisionParameters, details brokerapi.ProvisionDetails) *awss3.BucketDetails {
	bucketDetails := b.bucketFromPlan(servicePlan)
	bucketDetails.Tags = b.bucketTags("Created", details.ServiceID, details.PlanID, details.OrganizationGUID, details.SpaceGUID)
	bucketDetails.Policy = string(servicePlan.S3Properties.BucketPolicy)
	bucketDetails.Encryption = string(servicePlan.S3Properties.Encryption)
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
