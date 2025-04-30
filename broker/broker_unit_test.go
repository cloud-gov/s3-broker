package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

type mockBucket struct {
	name string
	arn  string

	describeDetails awss3.BucketDetails
	describeErr     error
}

func (b mockBucket) Describe(bucketname, partition string) (awss3.BucketDetails, error) {
	if b.describeErr != nil {
		return awss3.BucketDetails{}, b.describeErr
	}
	return b.describeDetails, nil
}

func (b mockBucket) Create(bucketName string, details awss3.BucketDetails) (string, error) {
	return "", errors.New("not implemented")
	// b.name = bucketName
	// b.arn = "aws:" + bucketName
	// return
}

func (b mockBucket) Modify(bucketName string, details awss3.BucketDetails) error {
	return errors.New("not implemented")
}

func (b mockBucket) Delete(bucketName string, deleteObjects bool) error {
	return errors.New("not implemented")
}

type mockCatalog struct {
	serviceName string
	planName    string
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
	if c.planName == "" {
		return ServicePlan{}, false
	}
	return ServicePlan{
		Name: c.planName,
	}, true
}

func (c mockCatalog) ListServicePlans() []ServicePlan {
	return nil
}

type mockUser struct {
	// In-memory state for tests.

	// accessKeys maps from usernames to lists of access key IDs. Note that a
	// new username is created for every binding, not every bucket.
	accessKeys           map[string][]string
	attachedUserPolicies []string
	deletedPolicyArns    []string
	detachedPolicyArns   []string
	exists               bool
	policies             []string // ARNs
	users                []string

	// Methods return these errors when set.
	attachUserPolicyErr         error
	createAccessKeyErr          error
	createPolicyErr             error
	createUserErr               error
	deleteAccessKeyErr          error
	deleteUserErr               error
	deleteUserPolicyErr         error
	detachUserPolicyErr         error
	listAccessKeysErr           error
	listAttachedUserPoliciesErr error
}

func (u *mockUser) ListAccessKeys(userName string) ([]string, error) {
	if u.listAccessKeysErr != nil {
		return []string{}, u.listAccessKeysErr
	}
	if u.accessKeys == nil {
		return []string{}, u.listAccessKeysErr
	}
	// Make ranging over the return value safe against changes to the slice,
	// like deletes.
	var out = make([]string, len(u.accessKeys[userName]))
	copy(out, u.accessKeys[userName])
	return out, nil
}

func (u *mockUser) ListAttachedUserPolicies(userName, iamPath string) ([]string, error) {
	if u.listAttachedUserPoliciesErr != nil {
		return []string{}, u.listAttachedUserPoliciesErr
	}
	return u.attachedUserPolicies, nil
}

func (u *mockUser) Delete(userName string) error {
	if u.deleteUserErr != nil {
		return u.deleteUserErr
	}
	u.exists = false
	return nil
}

func (u *mockUser) AttachUserPolicy(userName, policyARN string) error {
	if u.attachUserPolicyErr != nil {
		return u.attachUserPolicyErr
	}
	if u.attachedUserPolicies == nil {
		u.attachedUserPolicies = []string{}
	}
	u.attachedUserPolicies = append(u.attachedUserPolicies, policyARN)
	return nil
}

func (u *mockUser) Describe(userName string) (awsiam.UserDetails, error) {
	return awsiam.UserDetails{}, nil
}

func (u *mockUser) Create(userName, iamPath string, iamTags []*iam.Tag) (string, error) {
	if u.createUserErr != nil {
		return "", u.createUserErr
	}
	u.exists = true
	return "", nil
}

func (u *mockUser) CreateAccessKey(userName string) (string, string, error) {
	if u.createAccessKeyErr != nil {
		return "", "", u.createAccessKeyErr
	}
	if u.accessKeys == nil {
		u.accessKeys = make(map[string][]string)
	}
	keyID := fmt.Sprintf("%v-%v", userName, len(u.accessKeys[userName]))
	u.accessKeys[userName] = append(u.accessKeys[userName], keyID)

	return keyID, "", nil
}

func (u *mockUser) DeleteAccessKey(userName, accessKeyID string) error {
	if u.deleteAccessKeyErr != nil {
		return u.deleteAccessKeyErr
	}
	if u.accessKeys == nil || len(u.accessKeys[userName]) == 0 {
		return errors.New("not found")
	}
	keys := u.accessKeys[userName]
	idx := slices.Index(keys, accessKeyID)
	if idx == -1 {
		return errors.New("not found")
	}
	u.accessKeys[userName] = slices.Delete(keys, idx, idx+1)
	return nil
}

func (u *mockUser) CreatePolicy(policyName, iamPath, policyTemplate string, resources []string, iamTags []*iam.Tag) (string, error) {
	if u.createPolicyErr != nil {
		return "", u.createPolicyErr
	}
	if u.policies == nil {
		u.policies = []string{}
	}
	u.policies = append(u.policies, policyName)
	return policyName, nil

}

func (u *mockUser) DeletePolicy(policyARN string) error {
	if u.deleteUserPolicyErr != nil {
		return u.deleteUserPolicyErr
	}
	if u.policies == nil || len(u.policies) == 0 {
		return errors.New("not found")
	}
	idx := slices.Index(u.policies, policyARN)
	if idx == -1 {
		return errors.New("not found")
	}
	u.policies = slices.Delete(u.policies, idx, idx+1)
	return nil
}

func (u *mockUser) DetachUserPolicy(userName, policyARN string) error {
	if u.detachUserPolicyErr != nil {
		return u.detachUserPolicyErr
	}
	u.detachedPolicyArns = append(u.detachedPolicyArns, policyARN)
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
	logger := lager.NewLogger("broker-unit-test-TestUnbind")
	listAccessKeysErr := errors.New("list access keys error")
	deleteAccessKeyErr := errors.New("delete access key error")
	listAttachedUserPoliciesErr := errors.New("listing user policies error")
	detachUserPolicyErr := errors.New("detach user policy error")
	deleteUserPolicyErr := errors.New("delete user policy error")
	deleteUserErr := errors.New("delete user error")
	noSuchEntityErr := awserr.New("NoSuchEntity", "user does not exist", errors.New("original error"))

	testCases := map[string]struct {
		instanceId               string
		bindingId                string
		unbindDetails            domain.UnbindDetails
		broker                   *S3Broker
		expectedErr              error
		expectAccessKeys         map[string][]string
		expectDetachedPolicyArns []string
		expectPolicyARNs         []string
		expectUnbindSpec         domain.UnbindSpec
	}{
		"success": {
			instanceId:    "fake-instance-id",
			bindingId:     "fake-binding-id",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user:   &mockUser{},
			},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		// Add NoSuchEntity error on GetUser
		"NoSuchEntity error when deleting user": {
			instanceId:    "fake-instance-id",
			bindingId:     "deleted-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					deleteUserErr: noSuchEntityErr,
				},
				userPrefix: "test-user",
			},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"unexpected error deleting user": {
			instanceId:    "fake-instance-id",
			bindingId:     "deleted-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					deleteUserErr: deleteUserErr,
				},
			},
			expectUnbindSpec: domain.UnbindSpec{},
			expectedErr:      deleteUserErr,
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
			expectedErr:      listAccessKeysErr,
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"NoSuchEntity error listing access keys": {
			instanceId:    "fake-instance-id",
			bindingId:     "fake-binding-id",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					listAccessKeysErr: noSuchEntityErr,
				},
			},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"deletes access keys": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					accessKeys: map[string][]string{
						"prefix-binding-1": {"key1", "key2"},
					},
				},
				userPrefix: "prefix",
			},
			expectAccessKeys: map[string][]string{"prefix-binding-1": {}},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"error deleting access key": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					accessKeys: map[string][]string{
						"prefix-binding-1": {"key1"},
					},
					deleteAccessKeyErr: deleteAccessKeyErr,
				},
				userPrefix: "prefix",
			},
			expectedErr: deleteAccessKeyErr,
			expectAccessKeys: map[string][]string{
				"prefix-binding-1": {"key1"},
			},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"error listing user policies": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					listAttachedUserPoliciesErr: listAttachedUserPoliciesErr,
				},
				userPrefix: "prefix",
			},
			expectedErr:      listAttachedUserPoliciesErr,
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"NoSuchEntity error listing user policies": {
			instanceId:    "fake-instance-id",
			bindingId:     "fake-binding-id",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					listAttachedUserPoliciesErr: noSuchEntityErr,
				},
			},
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"error detaching user policy": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					attachedUserPolicies: []string{"policy1"},
					detachUserPolicyErr:  detachUserPolicyErr,
				},
			},
			expectedErr:      detachUserPolicyErr,
			expectUnbindSpec: domain.UnbindSpec{},
		},
		"detaches policy successfully and errors deleting policy": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					attachedUserPolicies: []string{"policy1"},
					deleteUserPolicyErr:  deleteUserPolicyErr,
				},
			},
			expectedErr:              deleteUserPolicyErr,
			expectDetachedPolicyArns: []string{"policy1"},
			expectUnbindSpec:         domain.UnbindSpec{},
		},
		"detaches policy and deletes policy successfully": {
			instanceId:    "fake-instance-id",
			bindingId:     "binding-1",
			unbindDetails: domain.UnbindDetails{},
			broker: &S3Broker{
				logger: logger,
				user: &mockUser{
					attachedUserPolicies: []string{"policy1"},
					policies:             []string{"policy1"},
				},
			},
			expectDetachedPolicyArns: []string{"policy1"},
			expectPolicyARNs:         []string{},
			expectUnbindSpec:         domain.UnbindSpec{},
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			unbindSpec, err := test.broker.Unbind(
				context.Background(),
				test.instanceId,
				test.bindingId,
				test.unbindDetails,
				false,
			)
			if !cmp.Equal(test.expectUnbindSpec, unbindSpec) {
				t.Fatalf(cmp.Diff(unbindSpec, test.expectUnbindSpec))
			}
			if user, ok := test.broker.user.(*mockUser); ok {
				if !cmp.Equal(test.expectAccessKeys, user.accessKeys) {
					t.Fatalf(cmp.Diff(user.accessKeys, test.expectAccessKeys))
				}
				if !cmp.Equal(test.expectDetachedPolicyArns, user.detachedPolicyArns) {
					t.Fatalf(cmp.Diff(user.detachedPolicyArns, test.expectDetachedPolicyArns))
				}
				if !cmp.Equal(test.expectPolicyARNs, user.policies) {
					t.Fatalf(cmp.Diff(user.policies, test.expectPolicyARNs))
				}
			}
			if err != test.expectedErr {
				t.Fatalf("expected error: %s, got: %s", test.expectedErr, err)
			}
		})
	}
}

// testErr is for checking errors returned by functions under test. Call NewTestErr with
// the desired error message, and call errors.Is(expected, actual) to compare the messages.
type testErr struct {
	msg string
}

func (e testErr) Error() string {
	return e.msg
}

func (e testErr) Is(target error) bool {
	return target.Error() == e.msg
}

func NewTestErr(msg string) error {
	return testErr{
		msg: msg,
	}
}

type MockProvider struct{}

func (p *MockProvider) Endpoint() string {
	return ""
}

func TestBind(t *testing.T) {
	logger := lager.NewLogger("broker-unit-test-TestBind")

	testCases := map[string]struct {
		// inputs
		instanceId  string
		bindingId   string
		bindDetails domain.BindDetails

		// state
		broker *S3Broker

		// outputs
		expectBinding domain.Binding
		expectErr     error

		// side effects
		expectUserExists         bool
		expectUser               mockUser // todo dedup with above
		expectAccessKeys         map[string][]string
		expectPolicies           []string
		expectAttachedPolicyArns []string
	}{
		"malformed bind parameters": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				RawParameters: json.RawMessage("{"),
			},
			broker: &S3Broker{
				logger: logger,
			},
			expectBinding: domain.Binding{},
			expectErr:     NewTestErr("unexpected end of JSON input"),
		},
		"missing service plan": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID: "plan1",
			},
			broker: &S3Broker{
				logger:  logger,
				catalog: &mockCatalog{},
			},
			expectBinding: domain.Binding{},
			expectErr:     NewTestErr("Service Plan 'plan1' not found"),
		},
		"missing service": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "plan1",
				ServiceID: "service1",
			},
			broker: &S3Broker{
				logger: logger,
				catalog: &mockCatalog{
					planName: "plan1",
				},
			},
			expectBinding: domain.Binding{},
			expectErr:     NewTestErr("Service 'service1' not found"),
		},
		"failed to create user": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "planid1",
				ServiceID: "serviceid1",
			},
			broker: &S3Broker{
				logger: logger,
				bucket: &mockBucket{
					describeDetails: awss3.BucketDetails{},
				},
				catalog: &mockCatalog{
					planName:    "plan1",
					serviceName: "service1",
				},
				tagManager: &mockTagGenerator{},
				user: &mockUser{
					createUserErr: NewTestErr("error creating user"),
				},
			},
			expectBinding:    domain.Binding{},
			expectErr:        NewTestErr("error creating user"),
			expectUserExists: false,
		},
		"failed to create access key": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "planid1",
				ServiceID: "serviceid1",
			},
			broker: &S3Broker{
				logger: logger,
				bucket: &mockBucket{
					describeDetails: awss3.BucketDetails{},
				},
				catalog: &mockCatalog{
					planName:    "plan1",
					serviceName: "service1",
				},
				tagManager: &mockTagGenerator{},
				user: &mockUser{
					createAccessKeyErr: NewTestErr("error creating access key"),
				},
			},
			expectBinding:    domain.Binding{},
			expectErr:        NewTestErr("error creating access key"),
			expectUserExists: false,
		},
		"failed to create policy": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "planid1",
				ServiceID: "serviceid1",
			},
			broker: &S3Broker{
				logger: logger,
				bucket: &mockBucket{
					describeDetails: awss3.BucketDetails{},
				},
				bucketPrefix: "test",
				catalog: &mockCatalog{
					planName:    "plan1",
					serviceName: "service1",
				},
				tagManager: &mockTagGenerator{},
				user: &mockUser{
					createPolicyErr: NewTestErr("error creating policy"),
				},
			},
			expectAccessKeys: map[string][]string{"-binding1": {}},
			expectBinding:    domain.Binding{},
			expectErr:        NewTestErr("error creating policy"),
			expectUserExists: false,
			expectPolicies:   nil,
		},
		"failed to attach policy": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "planid1",
				ServiceID: "serviceid1",
			},
			broker: &S3Broker{
				logger: logger,
				bucket: &mockBucket{
					describeDetails: awss3.BucketDetails{},
				},
				bucketPrefix: "test",
				catalog: &mockCatalog{
					planName:    "plan1",
					serviceName: "service1",
				},
				tagManager: &mockTagGenerator{},
				user: &mockUser{
					attachUserPolicyErr: NewTestErr("error attaching policy"),
				},
			},
			expectAccessKeys: map[string][]string{"-binding1": {}},
			expectBinding:    domain.Binding{},
			expectErr:        NewTestErr("error attaching policy"),
			expectUserExists: false,
			expectPolicies:   []string{},
		},
		"success": {
			instanceId: "instance1",
			bindingId:  "binding1",
			bindDetails: domain.BindDetails{
				PlanID:    "planid1",
				ServiceID: "serviceid1",
			},
			broker: &S3Broker{
				logger: logger,
				bucket: &mockBucket{
					describeDetails: awss3.BucketDetails{},
				},
				bucketPrefix: "test",
				catalog: &mockCatalog{
					planName:    "plan1",
					serviceName: "service1",
				},
				tagManager: &mockTagGenerator{},
				user:       &mockUser{},
			},
			expectAccessKeys: map[string][]string{"-binding1": {"-binding1-0"}},
			expectBinding: domain.Binding{
				Credentials: Credentials{
					URI:               "s3://-binding1-0:@/",
					AccessKeyID:       "-binding1-0",
					AdditionalBuckets: []string{""},
				},
			},
			expectUserExists: true,
			expectPolicies:   []string{"-binding1"},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// act
			binding, err := tc.broker.Bind(context.Background(), tc.instanceId, tc.bindingId, tc.bindDetails, false)

			// assert: outputs
			if !cmp.Equal(tc.expectBinding, binding) {
				t.Fatalf(cmp.Diff(binding, tc.expectBinding))
			}
			if !errors.Is(tc.expectErr, err) {
				t.Fatalf("expected err %s, got %s", tc.expectErr, err)
			}

			// assert: side effects
			if user, ok := tc.broker.user.(*mockUser); ok {
				if tc.expectUserExists != user.exists {
					t.Fatalf(cmp.Diff(user.exists, tc.expectUserExists))
				}
				if !cmp.Equal(tc.expectAccessKeys, user.accessKeys) {
					t.Fatalf(cmp.Diff(user.accessKeys, tc.expectAccessKeys))
				}
				if !cmp.Equal(tc.expectPolicies, user.policies) {
					t.Fatalf(cmp.Diff(user.policies, tc.expectPolicies))
				}
			}
		})
	}
}
