package awsiam

import (
	"errors"
)

type User interface {
	Describe(userName string) (UserDetails, error)
	Create(userName string) (string, error)
	Delete(userName string) error
	ListAccessKeys(userName string) ([]string, error)
	CreateAccessKey(userName string) (string, string, error)
	DeleteAccessKey(userName string, accessKeyID string) error
	CreatePolicy(policyName string, effect string, action string, resource string) (string, error)
	DeletePolicy(policyARN string) error
	ListAttachedUserPolicies(userName string) ([]string, error)
	AttachUserPolicy(userName string, policyARN string) error
	DetachUserPolicy(userName string, policyARN string) error
}

type UserDetails struct {
	UserName string
	UserARN  string
	UserID   string
}

var (
	ErrUserDoesNotExist = errors.New("iam user does not exist")
)
