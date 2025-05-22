package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	brokertags "github.com/cloud-gov/go-broker-tags"
	config "github.com/cloud-gov/s3-broker/cmd/tasks/config"
	tasksS3 "github.com/cloud-gov/s3-broker/cmd/tasks/s3"
	cf "github.com/cloudfoundry/go-cfclient/v3/client"
	cfconfig "github.com/cloudfoundry/go-cfclient/v3/config"

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

var (
	configFilePath string
)

func init() {
	flag.StringVar(&configFilePath, "config", "", "Location of the config file")
}

func run() error {
	actionPtr := flag.String("action", "", "Action to take. Accepted options: 'reconcile-tags'")
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
	log.Println("Jason1")
	var client *cf.Client
	if settings.CfApiUrl != "" && settings.CfApiClientId != "" && settings.CfApiClientSecret != "" {
		cfConfig, err := cfconfig.New(settings.CfApiUrl, cfconfig.ClientCredentials(settings.CfApiClientId, settings.CfApiClientSecret))
		if err != nil {
			log.Fatalf("Error creating CF config: %s", err)
		}
		client, err = cf.New(cfConfig)
		if err != nil {
			log.Fatalf("Error creating CF client: %s", err)
		}
	}
	log.Println("Jason2")
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(settings.Region),
	})
	if err != nil {
		return fmt.Errorf("could not initialize session: %s", err)
	}

	log.Println("Jason3")
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

		if slices.Contains(servicesToTag, "s3") {
			s3Client := s3.New(sess)
			err := tasksS3.ReconcileS3BucketTags(s3Client, tagManager, client)
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
