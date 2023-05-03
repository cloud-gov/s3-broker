package awss3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"

	"code.cloudfoundry.org/lager"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/exp/slices"
)

type S3Client interface {
	GetBucketLocation(input *s3.GetBucketLocationInput) (*s3.GetBucketLocationOutput, error)
	CreateBucket(input *s3.CreateBucketInput) (*s3.CreateBucketOutput, error)
	PutBucketTagging(input *s3.PutBucketTaggingInput) (*s3.PutBucketTaggingOutput, error)
	PutBucketEncryption(input *s3.PutBucketEncryptionInput) (*s3.PutBucketEncryptionOutput, error)
	PutBucketPolicy(input *s3.PutBucketPolicyInput) (*s3.PutBucketPolicyOutput, error)
	DeletePublicAccessBlock(input *s3.DeletePublicAccessBlockInput) (*s3.DeletePublicAccessBlockOutput, error)
	DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error)
}

type S3Bucket struct {
	s3svc  S3Client
	logger lager.Logger
}

type bucketPolicyStatement struct {
	Effect    string   `json:"Effect"`
	Principal string   `json:"Principal"`
	Action    []string `json:"Action"`
	Resource  []string `json:"Resource"`
}

type bucketPolicy struct {
	Version   string                  `json:"Version"`
	Statement []bucketPolicyStatement `json:"Statement"`
}

func NewS3Bucket(
	s3svc S3Client,
	logger lager.Logger,
) *S3Bucket {
	return &S3Bucket{
		s3svc:  s3svc,
		logger: logger.Session("s3-bucket"),
	}
}

func (s *S3Bucket) Describe(bucketName, partition string) (BucketDetails, error) {
	getLocationInput := &s3.GetBucketLocationInput{
		Bucket: aws.String(bucketName),
	}
	s.logger.Debug("get-bucket-location", lager.Data{"input": getLocationInput})

	getLocationOutput, err := s.s3svc.GetBucketLocation(getLocationInput)
	if err != nil {
		s.logger.Error("aws-s3-error", err)
		if awsErr, ok := err.(awserr.Error); ok {
			return BucketDetails{}, errors.New(awsErr.Code() + ": " + awsErr.Message())
		}
		return BucketDetails{}, err
	}
	s.logger.Debug("get-bucket-location", lager.Data{"output": getLocationOutput})

	region := getLocationOutput.LocationConstraint
	if region == nil {
		region = aws.String("us-east-1")
	}

	return s.buildBucketDetails(bucketName, *region, partition, nil), nil
}

// Create attempts to create an S3 bucket. If successful, it returns the bucket's location
// and a nil error. If not, it returns an empty string and an error.
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

	var tags []*s3.Tag
	for key, value := range bucketDetails.Tags {
		tags = append(tags, &s3.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	if _, err := s.s3svc.PutBucketTagging(&s3.PutBucketTaggingInput{
		Bucket: aws.String(bucketName),
		Tagging: &s3.Tagging{
			TagSet: tags,
		},
	}); err != nil {
		return "", err
	}

	if len(bucketDetails.Encryption) > 0 {
		var encryptionConfig s3.ServerSideEncryptionConfiguration
		if err := json.Unmarshal([]byte(bucketDetails.Encryption), &encryptionConfig); err != nil {
			return "", err
		}
		putEncryptionInput := &s3.PutBucketEncryptionInput{
			Bucket:                            aws.String(bucketName),
			ServerSideEncryptionConfiguration: &encryptionConfig,
		}
		s.logger.Debug("put-bucket-encryption", lager.Data{"input": putEncryptionInput})
		putEncryptionOutput, err := s.s3svc.PutBucketEncryption(putEncryptionInput)
		if err != nil {
			s.logger.Error("aws-s3-error", err)
			if awsErr, ok := err.(awserr.Error); ok {
				return "", errors.New(awsErr.Code() + ": " + awsErr.Message())
			}
			return "", err
		}
		s.logger.Debug("put-bucket-encryption", lager.Data{"output": putEncryptionOutput})
	}

	if err = s.checkDeletePublicAccessBlock(bucketDetails, bucketName); err != nil {
		return "", err
	}

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
			s.logger.Error("aws-s3-error putting bucket policy", err)
			if awsErr, ok := err.(awserr.Error); ok {
				return "", errors.New(awsErr.Code() + ": " + awsErr.Message())
			}
			return "", err
		}
		s.logger.Debug("put-bucket-policy", lager.Data{"output": putPolicyOutput})
	}

	return aws.StringValue(createBucketOutput.Location), nil
}

// checkDeletePublicAccessBlock checks the Policy of bucketDetails to see if the bucket
// is intended to be public. If so, it deletes the Public Access Block that is set on all
// new S3 buckets by default as of April 2023.
func (s *S3Bucket) checkDeletePublicAccessBlock(bucketDetails BucketDetails, bucketName string) error {
	// buckets with no policy are private by default.
	if bucketDetails.Policy == "" {
		return nil
	}

	var policy bucketPolicy
	err := json.Unmarshal([]byte(bucketDetails.Policy), &policy)
	if err != nil {
		s.logger.Error("aws-s3-error", err)
		return err
	}
	if len(policy.Statement) > 1 {
		err = fmt.Errorf("expected 1 policy statement, got %v", len(policy.Statement))
		s.logger.Error("aws-s3-error", err)
		return err
	}

	if isBucketPolicyPublic(policy.Statement) {
		s.logger.Debug("delete-public-access-block")
		_, err := s.s3svc.DeletePublicAccessBlock(&s3.DeletePublicAccessBlockInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			s.logger.Error("failed to delete public access block", err)
			return err
		}
	}
	return nil
}

func (s *S3Bucket) Modify(bucketName string, bucketDetails BucketDetails) error {
	// TODO Implement modifx
	return nil
}

func (s *S3Bucket) Delete(bucketName string, deleteObjects bool) error {

	deleteBucketInput := &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	}
	s.logger.Debug("delete-bucket", lager.Data{"input": deleteBucketInput})
	if deleteObjects {
		contentDeleteErr := s.deleteBucketContents(bucketName)
		if contentDeleteErr != nil {
			return contentDeleteErr
		}
	}
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

func (s *S3Bucket) deleteBucketContents(bucketName string) error {
	iter := s3manager.NewDeleteListIterator(s.s3svc.(*s3.S3), &s3.ListObjectsInput{
		Bucket: aws.String(bucketName),
	})

	if err := s3manager.NewBatchDeleteWithClient(s.s3svc.(*s3.S3)).Delete(aws.BackgroundContext(), iter); err != nil {
		s.logger.Error("aws-s3-error", err)
		if awsErr, ok := err.(awserr.Error); ok {
			return errors.New(awsErr.Code() + ": " + awsErr.Message())
		}
		return err
	}
	return nil
}

func (s3 *S3Bucket) buildBucketDetails(bucketName, region, partition string, attributes map[string]string) BucketDetails {
	return BucketDetails{
		BucketName:   bucketName,
		Region:       region,
		ARN:          fmt.Sprintf("arn:%s:s3:::%s", partition, bucketName),
		FIPSEndpoint: fmt.Sprintf("s3-fips.%s.amazonaws.com", region),
	}
}

func (s *S3Bucket) buildCreateBucketInput(bucketName string, bucketDetails BucketDetails) *s3.CreateBucketInput {
	createBucketInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}
	return createBucketInput
}

func isBucketPolicyPublic(policyStatements []bucketPolicyStatement) bool {
	publicAccessPolicy := bucketPolicyStatement{
		Effect:    "Allow",
		Principal: "*",
		Action:    []string{"s3:GetObject"},
	}
	return slices.ContainsFunc(policyStatements, func(statement bucketPolicyStatement) bool {
		return statement.Effect == publicAccessPolicy.Effect &&
			statement.Principal == publicAccessPolicy.Principal &&
			slices.Equal(statement.Action, publicAccessPolicy.Action)
	})
}
