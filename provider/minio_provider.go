package provider

type MinioProvider struct {
	endpoint string
}

func (m *MinioProvider) Endpoint() string {
	return m.endpoint
}
