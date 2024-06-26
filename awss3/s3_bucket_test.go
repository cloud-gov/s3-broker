package awss3

import (
	"errors"
	"fmt"
	"testing"

	"code.cloudfoundry.org/lager/v3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type MockS3Client struct {
	deletePublicAccessBlockCalled    bool
	numPutBucketPolicyCalls          int
	numPutBucketPolicyCallsShouldErr int
	putBucketPolicyErr               error
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
	c.numPutBucketPolicyCalls++
	if c.numPutBucketPolicyCalls <= c.numPutBucketPolicyCallsShouldErr {
		return nil, c.putBucketPolicyErr
	}
	return &s3.PutBucketPolicyOutput{}, nil
}

func (c *MockS3Client) DeletePublicAccessBlock(input *s3.DeletePublicAccessBlockInput) (*s3.DeletePublicAccessBlockOutput, error) {
	c.deletePublicAccessBlockCalled = true
	return &s3.DeletePublicAccessBlockOutput{}, nil
}

func (c *MockS3Client) DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	return nil, nil
}

func (c *MockS3Client) GetPublicAccessBlock(input *s3.GetPublicAccessBlockInput) (*s3.GetPublicAccessBlockOutput, error) {
	noPublicAccessBlockErr := awserr.New("NoSuchPublicAccessBlockConfiguration", "The public access block configuration was not found", errors.New("fail"))
	return &s3.GetPublicAccessBlockOutput{}, noPublicAccessBlockErr
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
		Name                                string
		BucketName                          string
		BucketDetails                       BucketDetails
		Location                            string
		Error                               error
		expectDeletePublicAccessBlockCalled bool
	}{
		{
			Name:       "basic bucket",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: "",
			},
			Location: "/b",
			Error:    nil,
		},
		{
			Name:       "public bucket",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: publicPolicy,
			},
			Location:                            "/b",
			Error:                               nil,
			expectDeletePublicAccessBlockCalled: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			mocks3Client := &MockS3Client{}
			b := NewS3Bucket(mocks3Client, lager.NewLogger("test"))
			location, err := b.Create(tc.BucketName, tc.BucketDetails)
			if location != tc.Location {
				t.Errorf("expected location %v, got %v", tc.Location, location)
			}
			if !errors.Is(err, tc.Error) {
				t.Errorf("expected return error %v, got %v", tc.Error, err)
			}
			if tc.expectDeletePublicAccessBlockCalled != mocks3Client.deletePublicAccessBlockCalled {
				t.Errorf("expected public access called: %v, got: %v", tc.expectDeletePublicAccessBlockCalled, mocks3Client.deletePublicAccessBlockCalled)
			}
		})
	}
}

func TestPutBucketPolicyWithRetries(t *testing.T) {
	accessDeniedErr := awserr.New("AccessDenied", "access denied", errors.New("original error"))
	unexpectedErr := errors.New("failure")

	cases := []struct {
		Name                            string
		BucketName                      string
		BucketDetails                   BucketDetails
		Location                        string
		Error                           error
		s3Client                        *MockS3Client
		expectedNumPutBucketPolicyCalls int
	}{
		{
			Name:       "success - public bucket",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: publicPolicy,
			},
			Location:                        "/b",
			Error:                           nil,
			s3Client:                        &MockS3Client{},
			expectedNumPutBucketPolicyCalls: 1,
		},
		{
			Name:       "success - public bucket with max allowed retries",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: publicPolicy,
			},
			Location:                        "/b",
			Error:                           nil,
			expectedNumPutBucketPolicyCalls: 11,
			s3Client: &MockS3Client{
				putBucketPolicyErr:               awserr.New("AccessDenied", "access denied", errors.New("original error")),
				numPutBucketPolicyCallsShouldErr: 10,
			},
		},
		{
			Name:       "failure - runs out of retries",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: publicPolicy,
			},
			Location:                        "/b",
			Error:                           accessDeniedErr,
			expectedNumPutBucketPolicyCalls: 11,
			s3Client: &MockS3Client{
				putBucketPolicyErr:               accessDeniedErr,
				numPutBucketPolicyCallsShouldErr: 11,
			},
		},
		{
			Name:       "failure - unexpected error",
			BucketName: "b",
			BucketDetails: BucketDetails{
				Policy: publicPolicy,
			},
			Location:                        "/b",
			Error:                           unexpectedErr,
			expectedNumPutBucketPolicyCalls: 1,
			s3Client: &MockS3Client{
				putBucketPolicyErr:               unexpectedErr,
				numPutBucketPolicyCallsShouldErr: 1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			b := NewS3Bucket(tc.s3Client, lager.NewLogger("test"))
			err := b.putBucketPolicyWithRetries(tc.BucketDetails, tc.BucketName)
			if !errors.Is(err, tc.Error) {
				t.Fatalf("expected return error %v, got %v", tc.Error, err)
			}
			if tc.s3Client.numPutBucketPolicyCalls != tc.expectedNumPutBucketPolicyCalls {
				t.Errorf("expected number of putBucketPolicyWithRetries calls: %d, got: %d", tc.expectedNumPutBucketPolicyCalls, tc.s3Client.numPutBucketPolicyCalls)
			}
		})
	}
}

func TestIsAccessDeniedException(t *testing.T) {
	isAccessDenied := isAccessDeniedException(awserr.New("AccessDenied", "access denied", errors.New("original error")))
	if !isAccessDenied {
		t.Fatal("expected isAccessDeniedException() to return true")
	}
	isAccessDenied = isAccessDeniedException(errors.New("random error"))
	if isAccessDenied {
		t.Fatal("expected isAccessDeniedException() to return false")
	}
}

func TestIsNoSuchBucketError(t *testing.T) {
	testCases := map[string]struct {
		inputErr                error
		expectIsNoSuchBucketErr bool
	}{
		"non-AWS error": {
			inputErr:                errors.New("fail"),
			expectIsNoSuchBucketErr: false,
		},
		"AWS NoSuchBucket error": {
			inputErr:                awserr.New("NoSuchBucket", "no such bucket", errors.New("original error")),
			expectIsNoSuchBucketErr: true,
		},
		"AWS random error": {
			inputErr:                awserr.New("RandomError", "access denied", errors.New("original error")),
			expectIsNoSuchBucketErr: false,
		},
		"AWS request failure error": {
			inputErr: awserr.NewRequestFailure(
				awserr.New("NoSuchBucket", "no such bucket", errors.New("original error")),
				404,
				"req-1",
			),
			expectIsNoSuchBucketErr: true,
		},
	}
	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			isNoSuchBucket := isNoSuchBucketError(test.inputErr)
			if isNoSuchBucket != test.expectIsNoSuchBucketErr {
				t.Fatalf("expected isNoSuchBucketError() to return %t, got: %t", test.expectIsNoSuchBucketErr, isNoSuchBucket)
			}
		})
	}
}

func TestHandleDeleteError(t *testing.T) {
	noSuchBucketErr := awserr.New("NoSuchBucket", "no such bucket", errors.New("original error"))
	awsOtherErr := awserr.New("OtherError", "other error", errors.New("original error"))
	nonAwsErr := errors.New("random error")
	requestFailureErr := awserr.NewRequestFailure(
		awserr.New("NoSuchBucket", "failed to perform batch operation", errors.New("fail")),
		404,
		"req-1",
	)
	batchDeleteErr := s3manager.NewBatchError(
		"BatchedDeleteIncomplete",
		"some objects have failed to be deleted.",
		[]s3manager.Error{
			{
				OrigErr: awserr.NewRequestFailure(awserr.New("NoSuchBucket", "specified bucket does not exist", nil), 404, "req-1"),
				Bucket:  aws.String("bucket"),
				Key:     aws.String("key"),
			},
		},
	)

	testCases := map[string]struct {
		inputErr    error
		expectedErr error
	}{
		"NoSuchBucket error, expect nil": {
			inputErr: noSuchBucketErr,
		},
		"other AWS error, expect error": {
			inputErr:    awsOtherErr,
			expectedErr: awsOtherErr,
		},
		"non-AWS error, expect error": {
			inputErr:    nonAwsErr,
			expectedErr: nonAwsErr,
		},
		"request failure wrapped error, expect nil": {
			inputErr: requestFailureErr,
		},
		"batch delete error, expect nil": {
			inputErr: batchDeleteErr,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			mocks3Client := &MockS3Client{}
			b := NewS3Bucket(mocks3Client, lager.NewLogger("test"))
			err := b.handleDeleteError(test.inputErr)
			if !errors.Is(err, test.expectedErr) {
				t.Errorf("expected return error %v, got %v", test.expectedErr, err)
			}
		})
	}
}
