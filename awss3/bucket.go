package awss3

import (
	"errors"
)

type Bucket interface {
	Describe(bucketName, partition string) (BucketDetails, error)
	Create(bucketName string, details BucketDetails) (string, error)
	Modify(bucketName string, details BucketDetails) error
	Delete(bucketName string) error
}

type BucketDetails struct {
	BucketName   string
	ARN          string
	Region       string
	Policy       string
	AwsPartition string
	Tags         map[string]string
}

var (
	ErrBucketDoesNotExist = errors.New("s3 bucket does not exist")
)
