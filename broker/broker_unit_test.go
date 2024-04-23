package broker

import (
	"context"
	"errors"
	"testing"

	"code.cloudfoundry.org/lager/v3"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	brokertags "github.com/cloud-gov/go-broker-tags"
	"github.com/cloud-gov/s3-broker/awsiam"
	"github.com/cloud-gov/s3-broker/awss3"
	"github.com/google/go-cmp/cmp"
	"github.com/pivotal-cf/brokerapi/v10"
	"github.com/pivotal-cf/brokerapi/v10/domain"
)

type mockTagGenerator struct {
	serviceName string
	generateErr error
	tags        map[string]string
}

func (mt *mockTagGenerator) GenerateTags(
	action brokertags.Action,
	serviceName string,
	servicePlanName string,
	resourceGUIDs brokertags.ResourceGUIDs,
	getMissingResources bool,
) (map[string]string, error) {
	if mt.generateErr != nil {
		return nil, mt.generateErr
	}
	if mt.serviceName != "" {
		if mt.tags == nil {
			mt.tags = make(map[string]string)
		}
		mt.tags["service name"] = mt.serviceName
	}
	return mt.tags, nil
}

type mockCatalog struct {
	serviceName   string
	getServiceErr error
}

func (c mockCatalog) Validate() error {
	return nil
}

func (c mockCatalog) FindService(serviceID string) (service Service, found bool) {
	if c.serviceName == "" {
		return Service{}, false
	}
	return Service{
		Name: c.serviceName,
	}, true
}

func (c mockCatalog) FindServicePlan(planID string) (plan ServicePlan, found bool) {
	return ServicePlan{}, false
}

func (c mockCatalog) ListServicePlans() []ServicePlan {
	return nil
}

type mockUser struct {
	deletedUser        string
	userAlreadyDeleted bool
	accessKeys         []string
	listAccessKeysErr  error
	deletedAccessKeys  map[string][]string
}

func (u *mockUser) ListAccessKeys(userName string) ([]string, error) {
	if len(u.accessKeys) > 0 {
		return u.accessKeys, u.listAccessKeysErr
	}
	return []string{}, u.listAccessKeysErr
}

func (u *mockUser) ListAttachedUserPolicies(userName, iamPath string) ([]string, error) {
	return []string{}, nil
}

func (u *mockUser) Delete(userName string) error {
	if userName == u.deletedUser {
		u.userAlreadyDeleted = true
		return awserr.New("NoSuchEntity", "no such user", errors.New("original error"))
	}
	return nil
}

func (u *mockUser) AttachUserPolicy(userName, policyARN string) error {
	return nil
}

func (u *mockUser) Describe(userName string) (awsiam.UserDetails, error) {
	return awsiam.UserDetails{}, nil
}

func (u *mockUser) Create(userName, iamPath string, iamTags []*iam.Tag) (string, error) {
	return "", nil
}

func (u *mockUser) CreateAccessKey(userName string) (string, string, error) {
	return "", "", nil
}

func (u *mockUser) DeleteAccessKey(userName, accessKeyID string) error {
	if u.deletedAccessKeys == nil {
		u.deletedAccessKeys = make(map[string][]string)
	}
	u.deletedAccessKeys[userName] = append(u.deletedAccessKeys[userName], accessKeyID)
	return nil
}

func (u *mockUser) CreatePolicy(policyName, iamPath, policyTemplate string, resources []string, iamTags []*iam.Tag) (string, error) {
	return "", nil
}

func (u *mockUser) DeletePolicy(policyARN string) error {
	return nil
}

func (u *mockUser) DetachUserPolicy(userName, policyARN string) error {
	return nil
}

func TestCreateBucket(t *testing.T) {
	testCases := map[string]struct {
		broker              *S3Broker
		expectedDetails     *awss3.BucketDetails
		servicePlan         ServicePlan
		instanceID          string
		provisionParameters ProvisionParameters
		provisionDetails    brokerapi.ProvisionDetails
		expectErr           bool
	}{
		"success": {
			broker: &S3Broker{
				awsPartition: "gov",
				catalog: &mockCatalog{
					serviceName: "service-1",
				},
				tagManager: &mockTagGenerator{
					serviceName: "service-1",
					tags: map[string]string{
						"foo": "bar",
					},
				},
			},
			servicePlan: ServicePlan{
				ID:   "plan-1",
				Name: "plan",
				S3Properties: S3Properties{
					BucketPolicy: "fake-policy",
					Encryption:   "fake-encryption",
				},
			},
			provisionParameters: ProvisionParameters{
				ObjectOwnership: "bucket-owner",
			},
			provisionDetails: brokerapi.ProvisionDetails{},
			expectedDetails: &awss3.BucketDetails{
				Policy:          "fake-policy",
				Encryption:      "fake-encryption",
				AwsPartition:    "gov",
				ObjectOwnership: "bucket-owner",
				Tags: map[string]string{
					"foo":          "bar",
					"service name": "service-1",
				},
			},
		},
		"service not found": {
			broker: &S3Broker{
				awsPartition: "gov",
				catalog:      &mockCatalog{},
				tagManager: &mockTagGenerator{
					tags: map[string]string{
						"foo": "bar",
					},
				},
			},
			servicePlan: ServicePlan{
				ID:   "plan-1",
				Name: "plan",
				S3Properties: S3Properties{
					BucketPolicy: "fake-policy",
					Encryption:   "fake-encryption",
				},
			},
			provisionParameters: ProvisionParameters{
				ObjectOwnership: "bucket-owner",
			},
			provisionDetails: brokerapi.ProvisionDetails{},
			expectErr:        true,
		},
		"generate tags error": {
			broker: &S3Broker{
				awsPartition: "gov",
				catalog:      &mockCatalog{},
				tagManager: &mockTagGenerator{
					generateErr: errors.New("generate tags error"),
					tags: map[string]string{
						"foo": "bar",
					},
				},
			},
			servicePlan: ServicePlan{
				ID:   "plan-1",
				Name: "plan",
				S3Properties: S3Properties{
					BucketPolicy: "fake-policy",
					Encryption:   "fake-encryption",
				},
			},
			provisionParameters: ProvisionParameters{
				ObjectOwnership: "bucket-owner",
			},
			provisionDetails: brokerapi.ProvisionDetails{},
			expectErr:        true,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			details, err := test.broker.createBucket(
				test.instanceID,
				test.servicePlan,
				test.provisionParameters,
				test.provisionDetails,
			)

			if err != nil && !test.expectErr {
				t.Fatal(err)
			}

			if test.expectErr && err == nil {
				t.Fatalf("expected error, received nil")
			}

			if !cmp.Equal(details, test.expectedDetails) {
				t.Errorf(cmp.Diff(details, test.expectedDetails))
			}
		})
	}
}

func TestUnbind(t *testing.T) {
	logger := lager.NewLogger("broker-unit-test")
	listAccessKeysErr := errors.New("list access keys error")

	testCases := map[string]struct {
		instanceId               string
		bindingId                string
		unbindDetails            domain.UnbindDetails
		broker                   *S3Broker
		expectUserAlreadyDeleted bool
		expectedErr              error
		expectDeletedAccessKeys  map[string][]string
	}{
		"success": {
			instanceId:    "fake-instance-id",
			bindingId:     "fake-binding-id",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user:   &mockUser{},
			},
		},
		"user was already deleted": {
			instanceId:    "fake-instance-id",
			bindingId:     "deleted-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					deletedUser: "test-user-deleted-1",
				},
				userPrefix: "test-user",
			},
			expectUserAlreadyDeleted: true,
		},
		"error listing access keys": {
			instanceId:    "fake-instance-id",
			bindingId:     "fake-binding-id",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					listAccessKeysErr: listAccessKeysErr,
				},
			},
			expectedErr: listAccessKeysErr,
		},
		"deletes access keys": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					accessKeys: []string{"key1"},
				},
				userPrefix: "prefix",
			},
			expectDeletedAccessKeys: map[string][]string{
				"prefix-binding-1": {"key1"},
			},
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := test.broker.Unbind(
				context.Background(),
				test.instanceId,
				test.bindingId,
				test.unbindDetails,
				false,
			)
			if user, ok := test.broker.user.(*mockUser); ok {
				if user.userAlreadyDeleted != test.expectUserAlreadyDeleted {
					t.Fatalf(cmp.Diff(user.userAlreadyDeleted, test.expectUserAlreadyDeleted))
				}
				if !cmp.Equal(test.expectDeletedAccessKeys, user.deletedAccessKeys) {
					t.Fatalf(cmp.Diff(user.deletedAccessKeys, test.expectDeletedAccessKeys))
				}
			}
			if err != test.expectedErr {
				t.Fatalf("expected error: %s, got: %s", test.expectedErr, err)
			}
		})
	}
}
