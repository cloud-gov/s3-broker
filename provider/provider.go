package provider

type Provider interface {
	Endpoint() string
}

func New(provider, region, endpoint string) Provider {
	if provider == "minio" {
		return &MinioProvider{
			endpoint: endpoint,
		}
	}
	return &AwsProvider{
		region: region,
	}
}
