package broker_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry-community/s3-broker/broker"
)

var _ = Describe("Broker", func() {
	var (
		broker S3Broker
	)

	Describe("GetBucketURI", func() {
		BeforeEach(func() {
			broker = S3Broker{}
		})

		It("builds the uri for a bucket in us-east-1", func() {
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-east-1",
				AccessKeyID:     "access-key",
				SecretAccessKey: "secret-key",
			})
			Expect(uri).To(Equal("s3://access-key:secret-key@s3.amazonaws.com/bucket"))
		})

		It("builds the uri for a bucket in not us-east-1", func() {
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-gov-west-1",
				AccessKeyID:     "access-key",
				SecretAccessKey: "secret-key",
			})
			Expect(uri).To(Equal("s3://access-key:secret-key@s3-us-gov-west-1.amazonaws.com/bucket"))
		})
	})
})
