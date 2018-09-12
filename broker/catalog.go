package broker

import (
	"errors"
	"fmt"

	"github.com/pivotal-cf/brokerapi"
)

type Catalog struct {
	Services []Service `yaml:"services,omitempty"`
}

type Service struct {
	ID              string                            `yaml:"id"`
	Name            string                            `yaml:"name"`
	Description     string                            `yaml:"description"`
	Bindable        bool                              `yaml:"bindable"`
	Tags            []string                          `yaml:"tags,omitempty"`
	PlanUpdatable   bool                              `yaml:"plan_updateable"`
	Plans           []ServicePlan                     `yaml:"plans"`
	Requires        []brokerapi.RequiredPermission    `yaml:"requires,omitempty"`
	Metadata        *brokerapi.ServiceMetadata        `yaml:"metadata,omitempty"`
	DashboardClient *brokerapi.ServiceDashboardClient `yaml:"dashboard_client,omitempty"`
}

type ServicePlan struct {
	ID           string                         `yaml:"id"`
	Name         string                         `yaml:"name"`
	Description  string                         `yaml:"description"`
	Free         bool                           `yaml:"free"`
	Metadata     *brokerapi.ServicePlanMetadata `yaml:"metadata,omitempty"`
	Durable      bool                           `yaml:"durable,omitempty"`
	S3Properties S3Properties                   `yaml:"s3_properties,omitempty"`
}

type S3Properties struct {
	IamPolicy    string `yaml:"iam_policy,omitempty"`
	BucketPolicy string `yaml:"bucket_policy,omitempty"`
	Encryption   string `yaml:"encryption,omitempty"`
}

func (c Catalog) Validate() error {
	for _, service := range c.Services {
		if err := service.Validate(); err != nil {
			return fmt.Errorf("Validating Services configuration: %s", err)
		}
	}

	return nil
}

func (c Catalog) FindService(serviceID string) (service Service, found bool) {
	for _, service := range c.Services {
		if service.ID == serviceID {
			return service, true
		}
	}

	return service, false
}

func (c Catalog) FindServicePlan(planID string) (plan ServicePlan, found bool) {
	for _, service := range c.Services {
		for _, plan := range service.Plans {
			if plan.ID == planID {
				return plan, true
			}
		}
	}

	return plan, false
}

func (c Catalog) ListServicePlans() []ServicePlan {
	var plans []ServicePlan
	for _, service := range c.Services {
		for _, plan := range service.Plans {
			plans = append(plans, plan)
		}
	}
	return plans
}

func (s Service) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("Must provide a non-empty ID (%+v)", s)
	}

	if s.Name == "" {
		return fmt.Errorf("Must provide a non-empty Name (%+v)", s)
	}

	if s.Description == "" {
		return fmt.Errorf("Must provide a non-empty Description (%+v)", s)
	}

	for _, servicePlan := range s.Plans {
		if err := servicePlan.Validate(); err != nil {
			return fmt.Errorf("Validating Plans configuration: %s", err)
		}
	}

	return nil
}

func (sp ServicePlan) Validate() error {
	if sp.ID == "" {
		return fmt.Errorf("Must provide a non-empty ID (%+v)", sp)
	}

	if sp.Name == "" {
		return fmt.Errorf("Must provide a non-empty Name (%+v)", sp)
	}

	if sp.Description == "" {
		return fmt.Errorf("Must provide a non-empty Description (%+v)", sp)
	}

	if err := sp.S3Properties.Validate(); err != nil {
		return fmt.Errorf("Validating S3 Properties configuration: %s", err)
	}

	return nil
}

func (eq S3Properties) Validate() error {
	if len(eq.IamPolicy) == 0 {
		return errors.New("Must provide a non-empty IAM Policy")
	}

	return nil
}
