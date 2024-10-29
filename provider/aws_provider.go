package provider

type AwsProvider struct {
	region string
}

func (a *AwsProvider) Endpoint() string {
	return "s3-fips." + a.region + ".amazonaws.com"
}
