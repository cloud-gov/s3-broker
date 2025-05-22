package main

import (
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
)

func run() error {
	actionPtr := flag.String("action", "", "Action to take. Accepted options: 'reconcile-tags'")
	flag.Parse()
	var settings config.Settings

	// Load settings from environment
	if err := settings.LoadFromEnv(); err != nil {
		return fmt.Errorf("there was an error loading settings: %w", err)
	}
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

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(settings.Region),
	})
	if err != nil {
		return fmt.Errorf("could not initialize session: %s", err)
	}


	if *actionPtr == "reconcile-tags" {
		log.Println("far")
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
		log.Println("near")
		s3Client := s3.New(sess)
		err = tasksS3.ReconcileS3BucketTags(s3Client, tagManager, client, settings.Environment)
		if err != nil {
			return err
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
