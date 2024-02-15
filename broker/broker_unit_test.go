package broker

import (
	"errors"
	"testing"

	brokertags "github.com/cloud-gov/go-broker-tags"
	"github.com/cloud-gov/s3-broker/awss3"
	"github.com/google/go-cmp/cmp"
	"github.com/pivotal-cf/brokerapi/v10"
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
