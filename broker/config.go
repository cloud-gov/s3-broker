package broker

import (
	"errors"
	"fmt"
)

type Config struct {
	Region                       string  `yaml:"region"`
	IamPath                      string  `yaml:"iam_path"`
	UserPrefix                   string  `yaml:"user_prefix"`
	PolicyPrefix                 string  `yaml:"policy_prefix"`
	BucketPrefix                 string  `yaml:"bucket_prefix"`
	AwsPartition                 string  `yaml:"aws_partition"`
	AllowUserProvisionParameters bool    `yaml:"allow_user_provision_parameters"`
	AllowUserUpdateParameters    bool    `yaml:"allow_user_update_parameters"`
	Catalog                      Catalog `yaml:"catalog"`
}

func (c Config) Validate() error {
	if c.Region == "" {
		return errors.New("Must provide a non-empty Region")
	}

	if c.UserPrefix == "" {
		return errors.New("Must provide a non-empty UserPrefix")
	}

	if c.PolicyPrefix == "" {
		return errors.New("Must provide a non-empty PolicyPrefix")
	}

	if c.BucketPrefix == "" {
		return errors.New("Must provide a non-empty BucketPrefix")
	}

	if c.AwsPartition == "" {
		return errors.New("Must provide a non-empty AwsPartition")
	}

	if err := c.Catalog.Validate(); err != nil {
		return fmt.Errorf("Validating Catalog configuration: %s", err)
	}

	return nil
}
