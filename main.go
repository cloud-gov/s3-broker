package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cloudfoundry-community/go-cfclient"
	"github.com/pivotal-cf/brokerapi"

	"github.com/cloudfoundry-community/s3-broker/awsiam"
	"github.com/cloudfoundry-community/s3-broker/awss3"
	"github.com/cloudfoundry-community/s3-broker/broker"
)

var (
	configFilePath string
	port           string

	logLevels = map[string]lager.LogLevel{
		"DEBUG": lager.DEBUG,
		"INFO":  lager.INFO,
		"ERROR": lager.ERROR,
		"FATAL": lager.FATAL,
	}
)

func init() {
	flag.StringVar(&configFilePath, "config", "", "Location of the config file")
	flag.StringVar(&port, "port", "3000", "Listen port")
}

func buildLogger(logLevel string) lager.Logger {
	laggerLogLevel, ok := logLevels[strings.ToUpper(logLevel)]
	if !ok {
		log.Fatal("Invalid log level: ", logLevel)
	}

	logger := lager.NewLogger("s3-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, laggerLogLevel))

	return logger
}

func main() {
	flag.Parse()

	config, err := LoadConfig(configFilePath)
	if err != nil {
		log.Fatalf("Error loading config file: %s", err)
	}

	logger := buildLogger(config.LogLevel)

	awsConfig := aws.NewConfig().WithRegion(config.S3Config.Region)
	awsSession := session.New(awsConfig)

	s3svc := s3.New(awsSession)
	s3bucket := awss3.NewS3Bucket(s3svc, logger)

	iamsvc := iam.New(awsSession)
	user := awsiam.NewIAMUser(iamsvc, logger)

	client, err := cfclient.NewClient(&config.CloudFoundry)
	if err != nil {
		log.Fatalf("Error creating CF client: %s", err)
	}

	serviceBroker := broker.New(config.S3Config, s3bucket, user, client, logger)

	credentials := brokerapi.BrokerCredentials{
		Username: config.Username,
		Password: config.Password,
	}

	brokerAPI := brokerapi.New(serviceBroker, logger, credentials)
	http.Handle("/", brokerAPI)

	fmt.Println("S3 Service Broker started on port " + port + "...")
	http.ListenAndServe(":"+port, nil)
}
