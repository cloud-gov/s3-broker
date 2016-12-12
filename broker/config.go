package broker

import (
	"errors"
	"fmt"
)

type Config struct {
	Region                       string  `json:"region"`
	IamPath                      string  `json:"iam_path"`
	BucketPrefix                 string  `json:"bucket_prefix"`
	AwsPartition                 string  `json:"aws_partition"`
	AllowUserProvisionParameters bool    `json:"allow_user_provision_parameters"`
	AllowUserUpdateParameters    bool    `json:"allow_user_update_parameters"`
	Catalog                      Catalog `json:"catalog"`
}

func (c Config) Validate() error {
	if c.Region == "" {
		return errors.New("Must provide a non-empty Region")
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
