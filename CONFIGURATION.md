# Configuration

A sample configuration can be found at [config-sample.yml](https://github.com/cloud-gov/s3-broker/blob/main/config-sample.yml).

## General Configuration

| Option    | Required | Type   | Description                                                                                                          |
| :-------- | :------: | :----- | :------------------------------------------------------------------------------------------------------------------- |
| log_level |    Y     | String | Broker Log Level (DEBUG, INFO, ERROR, FATAL)                                                                         |
| username  |    Y     | String | Broker Auth Username                                                                                                 |
| password  |    Y     | String | Broker Auth Password                                                                                                 |
| s3_config |    Y     | Hash   | [S3 Broker configuration](https://github.com/cloud-gov/s3-broker/blob/main/CONFIGURATION.md#s3-broker-configuration) |
| cf_config |    N     | Hash   | [Cloud Foundry configuration](https://godoc.org/github.com/cloudfoundry-community/go-cfclient#Config)                |

## S3 Broker Configuration

| Option                          | Required | Type    | Description                                                                                              |
| :------------------------------ | :------: | :------ | :------------------------------------------------------------------------------------------------------- |
| region                          |    Y     | String  | S3 Region                                                                                                |
| iam_path                        |    Y     | String  | IAM path                                                                                                 |
| user_prefix                     |    Y     | String  | IAM user name prefix                                                                                     |
| policy_prefix                   |    Y     | String  | IAM policy name prefix                                                                                   |
| bucket_prefix                   |    Y     | String  | Bucket name prefix                                                                                       |
| aws_partition                   |    Y     | String  | AWS partition (e.g. aws, aws-us-gov)                                                                     |
| allow_user_provision_parameters |    N     | Boolean | Allow users to send arbitrary parameters on provision calls (defaults to `false`)                        |
| allow_user_update_parameters    |    N     | Boolean | Allow users to send arbitrary parameters on update calls (defaults to `false`)                           |
| catalog                         |    Y     | Hash    | [S3 Broker catalog](https://github.com/cloud-gov/s3-broker/blob/main/CONFIGURATION.md#s3-broker-catalog) |

## S3 Broker catalog

Please refer to the [Catalog Documentation](https://docs.cloudfoundry.org/services/api.html#catalog-mgmt) for more details about these properties.

### Catalog

| Option   | Required | Type      | Description                                                                                     |
| :------- | :------: | :-------- | :---------------------------------------------------------------------------------------------- |
| services |    N     | []Service | A list of [Services](https://github.com/cloud-gov/s3-broker/blob/main/CONFIGURATION.md#service) |

### Service

| Option                        | Required | Type          | Description                                                                                                                 |
| :---------------------------- | :------: | :------------ | :-------------------------------------------------------------------------------------------------------------------------- |
| id                            |    Y     | String        | An identifier used to correlate this service in future requests to the catalog                                              |
| name                          |    Y     | String        | The CLI-friendly name of the service that will appear in the catalog. All lowercase, no spaces                              |
| description                   |    Y     | String        | A short description of the service that will appear in the catalog                                                          |
| bindable                      |    N     | Boolean       | Whether the service can be bound to applications                                                                            |
| tags                          |    N     | []String      | A list of service tags                                                                                                      |
| metadata.displayName          |    N     | String        | The name of the service to be displayed in graphical clients                                                                |
| metadata.imageUrl             |    N     | String        | The URL to an image                                                                                                         |
| metadata.longDescription      |    N     | String        | Long description                                                                                                            |
| metadata.providerDisplayName  |    N     | String        | The name of the upstream entity providing the actual service                                                                |
| metadata.documentationUrl     |    N     | String        | Link to documentation page for service                                                                                      |
| metadata.supportUrl           |    N     | String        | Link to support for the service                                                                                             |
| requires                      |    N     | []String      | A list of permissions that the user would have to give the service, if they provision it (only `syslog_drain` is supported) |
| plan_updateable               |    N     | Boolean       | Whether the service supports upgrade/downgrade for some plans                                                               |
| plans                         |    N     | []ServicePlan | A list of [Plans](https://github.com/cloud-gov/s3-broker/blob/main/CONFIGURATION.md#service-plan) for this service          |
| dashboard_client.id           |    N     | String        | The id of the Oauth2 client that the service intends to use                                                                 |
| dashboard_client.secret       |    N     | String        | A secret for the dashboard client                                                                                           |
| dashboard_client.redirect_uri |    N     | String        | A domain for the service dashboard that will be whitelisted by the UAA to enable SSO                                        |

### Service Plan

| Option               | Required | Type         | Description                                                                                                                                                                                         |
| :------------------- | :------: | :----------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| id                   |    Y     | String       | An identifier used to correlate this plan in future requests to the catalog                                                                                                                         |
| name                 |    Y     | String       | The CLI-friendly name of the plan that will appear in the catalog. All lowercase, no spaces                                                                                                         |
| description          |    Y     | String       | A short description of the plan that will appear in the catalog                                                                                                                                     |
| metadata.bullets     |    N     | []String     | Features of this plan, to be displayed in a bulleted-list                                                                                                                                           |
| metadata.costs       |    N     | Cost Object  | An array-of-objects that describes the costs of a service, in what currency, and the unit of measure                                                                                                |
| metadata.displayName |    N     | String       | Name of the plan to be display in graphical clients                                                                                                                                                 |
| free                 |    N     | Boolean      | This field allows the plan to be limited by the non_basic_services_allowed field in a Cloud Foundry Quota                                                                                           |
| deletable            |    N     | Boolean      | If true the bucket contents will be automatically removed when the service instance is deleted. If false (the default) an error will be raised if the bucket is not empty and the delete will fail. |
| s3_properties        |    Y     | S3Properties | [S3 Properties](https://github.com/cloud-gov/s3-broker/blob/main/CONFIGURATION.md#s3-properties)                                                                                                    |

## S3 Properties

Please refer to the [Amazon S3 Documentation](https://aws.amazon.com/documentation/s3/) for more details about these properties.

| Option | Required | Type | Description |
| :----- | :------: | :--- | :---------- |
