package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	awsRds "github.com/aws/aws-sdk-go/service/rds"
	brokertags "github.com/cloud-gov/go-broker-tags"
	. "github.com/cloud-gov/s3-broker"
	tasksS3 "github.com/cloud-gov/s3-broker/cmd/tasks/s3"
	"github.com/cloud-gov/s3-broker/config"
	"golang.org/x/exp/slices"
)

type serviceNames []string

// String is an implementation of the flag.Value interface
func (s *serviceNames) String() string {
	return fmt.Sprintf("%v", *s)
}

// Set is an implementation of the flag.Value interface
func (s *serviceNames) Set(value string) error {
	*s = append(*s, value)
	return nil
}

var servicesToTag serviceNames

func run() error {
	actionPtr := flag.String("action", "", "Action to take. Accepted options: 'reconcile-tags', 'reconcile-log-groups'")
	flag.Var(&servicesToTag, "service", "Specify AWS service whose instances should have tags updated. Accepted options: 's3'")
	flag.Parse()

	if *actionPtr == "" {
		log.Fatal("--action flag is required")
	}

	if len(servicesToTag) == 0 {
		return errors.New("--service argument is required. Specify --service multiple times to update tags for multiple services")
	}

	var settings config.Settings

	// Load settings from environment
	if err := settings.LoadFromEnv(); err != nil {
		return fmt.Errorf("there was an error loading settings: %w", err)
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(settings.Region),
	})
	if err != nil {
		return fmt.Errorf("could not initialize session: %s", err)
	}

	if *actionPtr == "reconcile-tags" {
		tagManager, err := brokertags.NewCFTagManager(
			"s3 broker",
			settings.Environment,
			settings.CfApiUrl,
			settings.CfApiClientId,
			settings.CfApiClientSecret,
		)
		if err != nil {
			return fmt.Errorf("could not initialize tag manager: %s", err)
		}

		path, _ := os.Getwd()
		c := catalog.InitCatalog(path)

		if slices.Contains(servicesToTag, "s3") {
			s3Client := s3.New(sess)
			err := tasksS3.ReconcileS3BucketTags(c, db, s3Client, tagManager)
			if err != nil {
				return err
			}
		}
	}

	if *actionPtr == "reconcile-log-groups" {
		logsClient := cloudwatchlogs.New(sess)

		if slices.Contains(servicesToTag, "rds") {
			rdsClient := awsRds.New(sess)
			err := rds.ReconcileRDSCloudwatchLogGroups(logsClient, rdsClient, settings.DbNamePrefix, db)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err.Error())
	}
}
