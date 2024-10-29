package broker_test

import (
	"code.cloudfoundry.org/lager/v3"
	brokertags "github.com/cloud-gov/go-broker-tags"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/cloud-gov/s3-broker/broker"
)

type mockTagGenerator struct {
	tags map[string]string
}

func (mt *mockTagGenerator) GenerateTags(
	action brokertags.Action,
	serviceName string,
	servicePlanName string,
	resourceGUIDs brokertags.ResourceGUIDs,
	getMissingResources bool,
) (map[string]string, error) {
	return mt.tags, nil
}

var _ = Describe("Broker", func() {
	Describe("GetBucketURI", func() {
		It("builds the uri for a bucket in us-east-1", func() {
			broker := New(
				Config{},
				nil,
				nil,
				nil,
				lager.NewLogger("s3-broker-test"),
				&mockTagGenerator{},
			)
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-east-1",
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3-fips.amazonaws.com/bucket"))
		})

		It("builds the uri for a bucket in not us-east-1", func() {
			broker := New(
				Config{},
				nil,
				nil,
				nil,
				lager.NewLogger("s3-broker-test"),
				&mockTagGenerator{},
			)
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-gov-west-1",
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3-fips.us-gov-west-1.amazonaws.com/bucket"))
		})
	})
})
