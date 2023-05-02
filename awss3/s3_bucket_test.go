package awss3_test

import (
	"errors"
	"fmt"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/cloudfoundry-community/s3-broker/awss3"
)

type MockS3Client struct {
}

func (c *MockS3Client) GetBucketLocation(input *s3.GetBucketLocationInput) (*s3.GetBucketLocationOutput, error) {
	return nil, nil
}

func (c *MockS3Client) CreateBucket(input *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	location := fmt.Sprint("/", *input.Bucket)
	return &s3.CreateBucketOutput{
		Location: &location,
	}, nil
}

func (c *MockS3Client) PutBucketTagging(input *s3.PutBucketTaggingInput) (*s3.PutBucketTaggingOutput, error) {
	return &s3.PutBucketTaggingOutput{}, nil
}

func (c *MockS3Client) PutBucketEncryption(input *s3.PutBucketEncryptionInput) (*s3.PutBucketEncryptionOutput, error) {
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (c *MockS3Client) PutBucketPolicy(input *s3.PutBucketPolicyInput) (*s3.PutBucketPolicyOutput, error) {
	return &s3.PutBucketPolicyOutput{}, nil
}

func (c *MockS3Client) DeletePublicAccessBlock(input *s3.DeletePublicAccessBlockInput) (*s3.DeletePublicAccessBlockOutput, error) {
	return &s3.DeletePublicAccessBlockOutput{}, nil
}

func (c *MockS3Client) DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	return nil, nil
}

var publicPolicy = `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": "*",
      "Action": ["s3:GetObject"],
      "Resource": ["arn:{{.AwsPartition}}:s3:::{{.BucketName}}/*"]
    }
  ]
}`

func TestCreate(t *testing.T) {
	cases := []struct {
		Name          string
		BucketName    string
		BucketDetails awss3.BucketDetails
		Location      string
		Error         error
	}{
		{
			Name:       "basic bucket",
			BucketName: "b",
			BucketDetails: awss3.BucketDetails{
				Policy: "",
			},
			Location: "/b",
			Error:    nil,
		},
		{
			Name:       "public bucket",
			BucketName: "b",
			BucketDetails: awss3.BucketDetails{
				Policy: publicPolicy,
			},
			Location: "/b",
			Error:    nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			b := awss3.NewS3Bucket(&MockS3Client{}, lager.NewLogger("test"))
			location, err := b.Create(tc.BucketName, tc.BucketDetails)
			if location != tc.Location {
				t.Errorf("expected location %v, got %v", tc.Location, location)
			}
			if !errors.Is(err, tc.Error) {
				t.Errorf("expected return error %v, got %v", tc.Error, err)
			}
		})
	}
}
