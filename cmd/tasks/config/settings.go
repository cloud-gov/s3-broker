package config

import (
	"errors"
	"fmt"
	"log"
	"os"
)

// Settings stores settings used to run the application
type Settings struct {
	EncryptionKey     string
	Environment       string
	Region            string
	CfApiUrl          string
	CfApiClientId     string
	CfApiClientSecret string
}

// LoadFromEnv loads settings from environment variables
func (s *Settings) LoadFromEnv() error {
	log.Println("Loading settings")

	// Ensure AWS credentials exist in environment
	for _, key := range []string{"AWS_DEFAULT_REGION"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("must set environment variable %s", key)
		}
	}

	// Load Encryption Key
	if _, ok := os.LookupEnv("ENC_KEY"); ok {
		s.EncryptionKey = os.Getenv("ENC_KEY")
	} else {
		return errors.New("an encryption key is required. Must specify ENC_KEY environment variable")
	}

	s.Environment = os.Getenv("ENVIRONMENT")

	s.Region = os.Getenv("AWS_DEFAULT_REGION")

	if cfApiUrl, ok := os.LookupEnv("CF_API_URL"); ok {
		s.CfApiUrl = cfApiUrl
	} else {
		return errors.New("CF_API_URL environment variable is required")
	}

	if cfApiClient, ok := os.LookupEnv("CF_API_CLIENT_ID"); ok {
		s.CfApiClientId = cfApiClient
	} else {
		return errors.New("CF_API_CLIENT_ID environment variable is required")
	}

	if cfApiClientSecret, ok := os.LookupEnv("CF_API_CLIENT_SECRET"); ok {
		s.CfApiClientSecret = cfApiClientSecret
	} else {
		return errors.New("CF_API_CLIENT_SECRET environment variable is required")
	}

	return nil
}
