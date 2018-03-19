package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry-community/s3-broker/broker"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LogLevel string        `yaml:"log_level"`
	Username string        `yaml:"username"`
	Password string        `yaml:"password"`
	S3Config broker.Config `yaml:"s3_config"`
	CFConfig *CFConfig     `yaml:"cf_config"`
}

type CFConfig struct {
	ApiAddress        string `yaml:"api_url"`
	Username          string `yaml:"user"`
	Password          string `yaml:"password"`
	ClientID          string `yaml:"client_id"`
	ClientSecret      string `yaml:"client_secret"`
	SkipSslValidation bool   `yaml:"skip_ssl_validation"`
	Token             string `yaml:"auth_token"`
	UserAgent         string `yaml:"user_agent"`
}

func LoadConfig(configFile string) (config *Config, err error) {
	if configFile == "" {
		return config, errors.New("Must provide a config file")
	}

	file, err := os.Open(configFile)
	if err != nil {
		return config, err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return config, err
	}

	if err = yaml.Unmarshal(bytes, &config); err != nil {
		return config, err
	}

	if err = config.Validate(); err != nil {
		return config, fmt.Errorf("Validating config contents: %s", err)
	}

	return config, nil
}

func (c Config) Validate() error {
	if c.LogLevel == "" {
		return errors.New("Must provide a non-empty LogLevel")
	}

	if c.Username == "" {
		return errors.New("Must provide a non-empty Username")
	}

	if c.Password == "" {
		return errors.New("Must provide a non-empty Password")
	}

	if err := c.S3Config.Validate(); err != nil {
		return fmt.Errorf("Validating S3 configuration: %s", err)
	}

	return nil
}
