package broker_test

import (
	"code.cloudfoundry.org/lager/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry-community/s3-broker/broker"
	"github.com/cloudfoundry-community/s3-broker/provider"
)

var _ = Describe("Broker", func() {
	Describe("GetBucketURI", func() {
		It("builds the uri for a bucket in us-east-1", func() {
			provider := provider.New("aws", "us-east-1", "")
			broker := New(
				Config{},
				provider,
				nil,
				nil,
				nil,
				lager.NewLogger("s3-broker-test"),
			)
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-east-1",
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3.amazonaws.com/bucket"))
		})

		It("builds the uri for a bucket in not us-east-1", func() {
			provider := provider.New("aws", "us-gov-west-1", "")
			broker := New(
				Config{},
				provider,
				nil,
				nil,
				nil,
				lager.NewLogger("s3-broker-test"),
			)
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-gov-west-1",
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3-us-gov-west-1.amazonaws.com/bucket"))
		})
	})
})
