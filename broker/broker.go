package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"code.cloudfoundry.org/lager/v3"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi/v10"
	"github.com/pivotal-cf/brokerapi/v10/domain"
	"github.com/pivotal-cf/brokerapi/v10/domain/apiresponses"

	"github.com/cloudfoundry-community/s3-broker/awsiam"
	"github.com/cloudfoundry-community/s3-broker/awss3"
	"github.com/cloudfoundry-community/s3-broker/provider"

	brokertags "github.com/cloud-gov/go-broker-tags"
)

const instanceIDLogKey = "instance-id"
const bindingIDLogKey = "binding-id"
const detailsLogKey = "details"
const acceptsIncompleteLogKey = "acceptsIncomplete"

var (
	ErrNoClientConfigured = errors.New("This broker is not configured to support binding to additional instances. Contact your Cloud Foundry operator for details.")
)

type S3Broker struct {
	provider                     provider.Provider
	insecureSkipVerify           bool
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
	tagManager                   brokertags.TagManager
}

type CatalogExternal struct {
	Services []brokerapi.Service `json:"services"`
}

type Credentials struct {
	URI                string   `json:"uri"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify"`
	AccessKeyID        string   `json:"access_key_id"`
	SecretAccessKey    string   `json:"secret_access_key"`
	Region             string   `json:"region"`
	Bucket             string   `json:"bucket"`
	Endpoint           string   `json:"endpoint"`
	FIPSEndpoint       string   `json:"fips_endpoint"`
	AdditionalBuckets  []string `json:"additional_buckets"`
}

func New(
	config Config,
	provider provider.Provider,
	bucket awss3.Bucket,
	user awsiam.User,
	cfClient *cfclient.Client,
	logger lager.Logger,
	tagManager brokertags.TagManager,
) *S3Broker {
	return &S3Broker{
		provider:                     provider,
		insecureSkipVerify:           config.InsecureSkipVerify,
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
	details domain.ProvisionDetails,
	asyncAllowed bool,
) (domain.ProvisionedServiceSpec, error) {
	b.logger.Debug("provision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	provisionParameters := ProvisionParameters{
		// Default object ownership to "ObjectWriter" so that ACLs can be used.
		// Preserves backwards compatibility after AWS changes:
		//   https://aws.amazon.com/blogs/aws/heads-up-amazon-s3-security-changes-are-coming-in-april-of-2023/
		ObjectOwnership: s3.ObjectOwnershipObjectWriter,
	}
	if b.allowUserProvisionParameters && len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &provisionParameters); err != nil {
			return domain.ProvisionedServiceSpec{}, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return domain.ProvisionedServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	instance, err := b.createBucket(instanceID, servicePlan, provisionParameters, details)
	if err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}
	if _, err = b.bucket.Create(b.bucketName(instanceID), *instance); err != nil {
		return domain.ProvisionedServiceSpec{}, err
	}

	return domain.ProvisionedServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) Update(
	context context.Context,
	instanceID string,
	details domain.UpdateDetails,
	asyncAllowed bool,
) (domain.UpdateServiceSpec, error) {
	b.logger.Debug("update", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	updateParameters := UpdateParameters{}
	if b.allowUserUpdateParameters && len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &updateParameters); err != nil {
			return domain.UpdateServiceSpec{}, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return domain.UpdateServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	instance := b.modifyBucket(instanceID, servicePlan, updateParameters, details)
	if err := b.bucket.Modify(b.bucketName(instanceID), *instance); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return domain.UpdateServiceSpec{}, apiresponses.ErrInstanceDoesNotExist
		}
		return domain.UpdateServiceSpec{}, err
	}

	return domain.UpdateServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) Deprovision(
	context context.Context,
	instanceID string,
	details domain.DeprovisionDetails,
	asyncAllowed bool,
) (domain.DeprovisionServiceSpec, error) {
	b.logger.Debug("deprovision", lager.Data{
		instanceIDLogKey:        instanceID,
		detailsLogKey:           details,
		acceptsIncompleteLogKey: asyncAllowed,
	})

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return domain.DeprovisionServiceSpec{}, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}
	if err := b.bucket.Delete(b.bucketName(instanceID), servicePlan.PlanDeletable); err != nil {
		if err == awss3.ErrBucketDoesNotExist {
			return domain.DeprovisionServiceSpec{}, brokerapi.ErrInstanceDoesNotExist
		}
		return domain.DeprovisionServiceSpec{}, err
	}

	return domain.DeprovisionServiceSpec{IsAsync: false}, nil
}

func (b *S3Broker) GetBucketURI(credentials Credentials) string {
	return fmt.Sprintf(
		"s3://%s:%s@%s/%s",
		url.QueryEscape(credentials.AccessKeyID),
		url.QueryEscape(credentials.SecretAccessKey),
		b.provider.Endpoint(),
		credentials.Bucket,
	)
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
	instanceID string,
	bindingID string,
	details domain.BindDetails,
	asyncAllowed bool,
) (domain.Binding, error) {
	b.logger.Debug("bind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	binding := domain.Binding{}

	var accessKeyID, secretAccessKey string
	var policyARN string
	var err error

	bindParameters := BindParameters{}
	if len(details.RawParameters) > 0 {
		if err := json.Unmarshal(details.RawParameters, &bindParameters); err != nil {
			return binding, err
		}
	}

	servicePlan, ok := b.catalog.FindServicePlan(details.PlanID)
	if !ok {
		return binding, fmt.Errorf("Service Plan '%s' not found", details.PlanID)
	}

	service, ok := b.catalog.FindService(details.ServiceID)
	if !ok {
		return binding, fmt.Errorf("Service '%s' not found", details.ServiceID)
	}

	tags, err := b.tagManager.GenerateTags(
		brokertags.Create,
		service.Name,
		servicePlan.Name,
		brokertags.ResourceGUIDs{
			InstanceGUID: instanceID,
		},
		true,
	)
	iamTags := awsiam.ConvertTagsMapToIAMTags(tags)

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
					errc <- apiresponses.ErrInstanceDoesNotExist
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
				credentials.FIPSEndpoint = bucketDetails.FIPSEndpoint
				credentials.Endpoint = b.provider.Endpoint()
				credentials.InsecureSkipVerify = b.insecureSkipVerify
			} else {
				credentials.AdditionalBuckets = append(credentials.AdditionalBuckets, bucketDetails.BucketName)
			}
		case err := <-errc:
			return binding, err
		}
	}

	if _, err = b.user.Create(b.userName(bindingID), b.iamPath, iamTags); err != nil {
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

	policyARN, err = b.user.CreatePolicy(
		b.policyName(bindingID),
		b.iamPath,
		string(servicePlan.S3Properties.IamPolicy),
		bucketARNs,
		iamTags,
	)
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
	details domain.UnbindDetails,
	asyncAllowed bool,
) (domain.UnbindSpec, error) {
	b.logger.Debug("unbind", lager.Data{
		instanceIDLogKey: instanceID,
		bindingIDLogKey:  bindingID,
		detailsLogKey:    details,
	})

	accessKeys, err := b.user.ListAccessKeys(b.userName(bindingID))
	if err != nil {
		return domain.UnbindSpec{}, err
	}

	for _, accessKey := range accessKeys {
		if err := b.user.DeleteAccessKey(b.userName(bindingID), accessKey); err != nil {
			return domain.UnbindSpec{}, err
		}
	}

	userPolicies, err := b.user.ListAttachedUserPolicies(b.userName(bindingID), b.iamPath)
	if err != nil {
		return domain.UnbindSpec{}, err
	}

	for _, userPolicy := range userPolicies {
		if err := b.user.DetachUserPolicy(b.userName(bindingID), userPolicy); err != nil {
			return domain.UnbindSpec{}, err
		}

		if err := b.user.DeletePolicy(userPolicy); err != nil {
			return domain.UnbindSpec{}, err
		}
	}

	if err := b.user.Delete(b.userName(bindingID)); err != nil {
		return domain.UnbindSpec{}, err
	}

	return domain.UnbindSpec{
		IsAsync: false,
	}, nil
}

func (b *S3Broker) LastOperation(
	ctx context.Context,
	instanceID string,
	details domain.PollDetails,
) (domain.LastOperation, error) {
	b.logger.Debug("last-operation", lager.Data{
		instanceIDLogKey: instanceID,
	})
	return domain.LastOperation{}, errors.New("this broker does not support LastOperation")
}

func (b *S3Broker) GetBinding(
	ctx context.Context,
	instanceID,
	bindingID string,
	details domain.FetchBindingDetails,
) (domain.GetBindingSpec, error) {
	b.logger.Debug("get-binding", lager.Data{
		instanceIDLogKey: instanceID,
	})
	return domain.GetBindingSpec{}, errors.New("this broker does not support GetBinding")
}

func (b *S3Broker) GetInstance(
	ctx context.Context,
	instanceID string,
	details domain.FetchInstanceDetails,
) (domain.GetInstanceDetailsSpec, error) {
	b.logger.Debug("get-instance", lager.Data{
		instanceIDLogKey: instanceID,
	})
	return domain.GetInstanceDetailsSpec{}, errors.New("this broker does not support GetInstance")
}

func (b *S3Broker) LastBindingOperation(
	ctx context.Context,
	instanceID,
	bindingID string,
	details domain.PollDetails,
) (domain.LastOperation, error) {
	b.logger.Debug("last-binding-operation", lager.Data{
		instanceIDLogKey: instanceID,
	})
	return domain.LastOperation{}, errors.New("this broker does not support LastBindingOperation")
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

func (b *S3Broker) createBucket(
	instanceID string,
	servicePlan ServicePlan,
	provisionParameters ProvisionParameters,
	details brokerapi.ProvisionDetails,
) (*awss3.BucketDetails, error) {
	bucketDetails := b.bucketFromPlan(servicePlan)

	service, ok := b.catalog.FindService(details.ServiceID)
	if !ok {
		return nil, fmt.Errorf("Service '%s' not found", details.ServiceID)
	}

	tags, err := b.tagManager.GenerateTags(
		brokertags.Create,
		service.Name,
		servicePlan.Name,
		brokertags.ResourceGUIDs{
			OrganizationGUID: details.OrganizationGUID,
			SpaceGUID:        details.SpaceGUID,
			InstanceGUID:     instanceID,
		},
		false,
	)
	if err != nil {
		return nil, err
	}
	bucketDetails.Tags = tags

	bucketDetails.Policy = string(servicePlan.S3Properties.BucketPolicy)
	bucketDetails.Encryption = string(servicePlan.S3Properties.Encryption)
	bucketDetails.AwsPartition = b.awsPartition
	bucketDetails.ObjectOwnership = provisionParameters.ObjectOwnership
	return bucketDetails, nil
}

func (b *S3Broker) modifyBucket(instanceID string, servicePlan ServicePlan, updateParameters UpdateParameters, details brokerapi.UpdateDetails) *awss3.BucketDetails {
	bucketDetails := b.bucketFromPlan(servicePlan)
	return bucketDetails
}

func (b *S3Broker) bucketFromPlan(servicePlan ServicePlan) *awss3.BucketDetails {
	bucketDetails := &awss3.BucketDetails{}
	return bucketDetails
}
