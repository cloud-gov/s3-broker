package broker

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pivotal-cf/brokerapi"
)

type Catalog struct {
	Services []Service `json:"services,omitempty"`
}

type Service struct {
	ID              string                            `json:"id"`
	Name            string                            `json:"name"`
	Description     string                            `json:"description"`
	Bindable        bool                              `json:"bindable"`
	Tags            []string                          `json:"tags,omitempty"`
	PlanUpdatable   bool                              `json:"plan_updateable"`
	Plans           []ServicePlan                     `json:"plans"`
	Requires        []brokerapi.RequiredPermission    `json:"requires,omitempty"`
	Metadata        *brokerapi.ServiceMetadata        `json:"metadata,omitempty"`
	DashboardClient *brokerapi.ServiceDashboardClient `json:"dashboard_client,omitempty"`
}

type ServicePlan struct {
	ID           string                         `json:"id"`
	Name         string                         `json:"name"`
	Description  string                         `json:"description"`
	Free         bool                           `json:"free"`
	Metadata     *brokerapi.ServicePlanMetadata `json:"metadata,omitempty"`
	S3Properties S3Properties                   `json:"s3_properties,omitempty"`
}

type S3Properties struct {
	IamPolicy    json.RawMessage `json:"iam_policy,omitempty"`
	BucketPolicy json.RawMessage `json:"bucket_policy,omitempty"`
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
