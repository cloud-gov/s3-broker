package broker

type ProvisionParameters struct {
}

type BindParameters struct {
	AdditionalInstances []string `mapstructure:"additional_instances"`
}

type UpdateParameters struct {
	ApplyImmediately bool `mapstructure:"apply_immediately"`
}
