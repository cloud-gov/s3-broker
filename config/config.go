package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloud-gov/s3-broker/broker"
	"gopkg.in/yaml.v2"
)

type Config struct {
	LogLevel    string        `yaml:"log_level"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	Environment string        `yaml:"environment"`
	S3Config    broker.Config `yaml:"s3_config"`
	CFConfig    *CFConfig     `yaml:"cf_config"`
}

type CFConfig struct {
	ApiAddress   string `yaml:"api_url"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
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
