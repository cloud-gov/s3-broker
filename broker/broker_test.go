package broker

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3.amazonaws.com/bucket"))
		})

		It("builds the uri for a bucket in not us-east-1", func() {
			uri := broker.GetBucketURI(Credentials{
				Bucket:          "bucket",
				Region:          "us-gov-west-1",
				AccessKeyID:     "access-key!",
				SecretAccessKey: "secret-key!",
			})
			Expect(uri).To(Equal("s3://access-key%21:secret-key%21@s3-us-gov-west-1.amazonaws.com/bucket"))
		})
	})

	Describe("getBucketTags", func() {
		It("builds the correct bucket tags", func() {
			tags := getBucketTags("Created", "abc1", "abc2", "abc3", "abc4", "abc5")
			delete(tags, "Created at")
			Expect(tags).To(Equal(map[string]string{
				"Owner":             "Cloud Foundry",
				"Created by":        "AWS S3 Service Broker",
				"Service GUID":      "abc1",
				"Plan GUID":         "abc2",
				"Organization GUID": "abc3",
				"Space GUID":        "abc4",
				"Instance GUID":     "abc5",
			}))
		})
	})
})
