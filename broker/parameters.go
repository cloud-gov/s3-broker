package broker

type ProvisionParameters struct {
	ObjectOwnership string `json:"object_ownership"`
}

type BindParameters struct {
	// Set AdditionalInstances to bind credentials that have permission to
	// access multiple s3 buckets, not just one. This is useful when copying
	// files between buckets. The contents should be a list of service
	// instance names.
	AdditionalInstances []string `json:"additional_instances"`
}

type UpdateParameters struct {
	ApplyImmediately bool `json:"apply_immediately"`
}
