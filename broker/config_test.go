package broker_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry-community/s3-broker/broker"
)

var _ = Describe("Config", func() {
	var (
		config Config

		validConfig = Config{
			Region:       "s3-region",
			UserPrefix:   "cf",
			PolicyPrefix: "cf",
			BucketPrefix: "cf",
			Catalog: Catalog{
				[]Service{
					Service{
						ID:          "service-1",
						Name:        "Service 1",
						Description: "Service 1 description",
					},
				},
			},
		}
	)

	Describe("Validate", func() {
		BeforeEach(func() {
			config = validConfig
		})

		It("does not return error if all sections are valid", func() {
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns error if Region is not valid", func() {
			config.Region = ""

			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Must provide a non-empty Region"))
		})

		It("returns error if UserPrefix is not valid", func() {
			config.UserPrefix = ""

			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Must provide a non-empty UserPrefix"))
		})

		It("returns error if PolicyPrefix is not valid", func() {
			config.PolicyPrefix = ""

			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Must provide a non-empty PolicyPrefix"))
		})

		It("returns error if BucketPrefix is not valid", func() {
			config.BucketPrefix = ""

			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Must provide a non-empty BucketPrefix"))
		})

		It("returns error if Catalog is not valid", func() {
			config.Catalog = Catalog{
				[]Service{
					Service{},
				},
			}

			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Validating Catalog configuration"))
		})
	})
})
