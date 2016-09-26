package broker

type ProvisionParameters struct {
}

type UpdateParameters struct {
	ApplyImmediately bool `mapstructure:"apply_immediately"`
}
