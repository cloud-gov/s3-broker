package awsiam

import (
	"errors"
	"fmt"

	"code.cloudfoundry.org/lager/v3"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

type User interface {
	Exists(userName string) (bool, error)
	Describe(userName string) (UserDetails, error)
	Create(userName, iamPath string, iamTags []*iam.Tag) (string, error)
	Delete(userName string) error
	ListAccessKeys(userName string) ([]string, error)
	CreateAccessKey(userName string) (string, string, error)
	DeleteAccessKey(userName, accessKeyID string) error
	CreatePolicy(policyName, iamPath, policyTemplate string, resources []string, iamTags []*iam.Tag) (string, error)
	DeletePolicy(policyARN string) error
	ListAttachedUserPolicies(userName, iamPath string) ([]string, error)
	AttachUserPolicy(userName, policyARN string) error
	DetachUserPolicy(userName, policyARN string) error
}

type UserDetails struct {
	UserName string
	UserARN  string
	UserID   string
}

var (
	ErrUserDoesNotExist = errors.New("iam user does not exist")
)

func NewUser(provider string, logger lager.Logger, awsSession *session.Session, endpoint string, insecureSkipVerify bool) (User, error) {
	fmt.Printf("Setting up AWS IAM user provider...\n")
	iamsvc := iam.New(awsSession)
	user := NewIAMUser(iamsvc, logger)
	return user, nil
}
