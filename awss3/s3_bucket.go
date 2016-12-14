package awss3

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"code.cloudfoundry.org/lager"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Bucket struct {
	s3svc  *s3.S3
	logger lager.Logger
}

func NewS3Bucket(
	s3svc *s3.S3,
	logger lager.Logger,
) *S3Bucket {
	return &S3Bucket{
		s3svc:  s3svc,
		logger: logger.Session("s3-bucket"),
	}
}

func (s *S3Bucket) Describe(bucketName, partition string) (BucketDetails, error) {
	//bucketDetails := BucketDetails{}

	return s.buildBucketDetails(bucketName, partition, nil), nil
}

func (s *S3Bucket) Create(bucketName string, bucketDetails BucketDetails) (string, error) {
	createBucketInput := s.buildCreateBucketInput(bucketName, bucketDetails)
	s.logger.Debug("create-bucket", lager.Data{"input": createBucketInput})

	createBucketOutput, err := s.s3svc.CreateBucket(createBucketInput)
	if err != nil {
		s.logger.Error("aws-s3-error", err)
		if awsErr, ok := err.(awserr.Error); ok {
			return "", errors.New(awsErr.Code() + ": " + awsErr.Message())
		}
		return "", err
	}
	s.logger.Debug("create-bucket", lager.Data{"output": createBucketOutput})

	if len(bucketDetails.Policy) > 0 {
		bucketDetails.BucketName = bucketName
		tmpl, err := template.New("policy").Parse(bucketDetails.Policy)
		if err != nil {
			s.logger.Error("aws-s3-error", err)
			return "", err
		}
		policy := bytes.Buffer{}
		err = tmpl.Execute(&policy, bucketDetails)
		if err != nil {
			s.logger.Error("aws-s3-error", err)
			return "", err
		}
		putPolicyInput := &s3.PutBucketPolicyInput{
			Bucket: aws.String(bucketDetails.BucketName),
			Policy: aws.String(policy.String()),
		}
		s.logger.Debug("put-bucket-policy", lager.Data{"input": putPolicyInput})
		putPolicyOutput, err := s.s3svc.PutBucketPolicy(putPolicyInput)
		if err != nil {
			s.logger.Error("aws-s3-error", err)
			if awsErr, ok := err.(awserr.Error); ok {
				return "", errors.New(awsErr.Code() + ": " + awsErr.Message())
			}
			return "", err
		}
		s.logger.Debug("put-bucket-policy", lager.Data{"output": putPolicyOutput})
	}

	return aws.StringValue(createBucketOutput.Location), nil
}

func (s *S3Bucket) Modify(bucketName string, bucketDetails BucketDetails) error {
	// TODO Implement modifx
	return nil
}

func (s *S3Bucket) Delete(bucketName string) error {

	deleteBucketInput := &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}
	s.logger.Debug("delete-bucket", lager.Data{"input": deleteBucketInput})

	deleteBucketOutput, err := s.s3svc.DeleteBucket(deleteBucketInput)
	if err != nil {
		s.logger.Error("aws-s3-error", err)
		if awsErr, ok := err.(awserr.Error); ok {
			if reqErr, ok := err.(awserr.RequestFailure); ok {
				// AWS S3 returns a 400 if Bucket is not found
				if reqErr.StatusCode() == 400 || reqErr.StatusCode() == 404 {
					return ErrBucketDoesNotExist
				}
			}
			return errors.New(awsErr.Code() + ": " + awsErr.Message())
		}
		return err
	}
	s.logger.Debug("delete-bucket", lager.Data{"output": deleteBucketOutput})

	return nil
}

func (s3 *S3Bucket) buildBucketDetails(bucketName, partition string, attributes map[string]string) BucketDetails {
	bucketDetails := BucketDetails{
		BucketName: bucketName,
		ARN:        fmt.Sprintf("arn:%s:s3:::%s", partition, bucketName),
	}
	return bucketDetails
}

func (s *S3Bucket) buildCreateBucketInput(bucketName string, bucketDetails BucketDetails) *s3.CreateBucketInput {
	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}
	return createBucketInput
}
