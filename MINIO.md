# MinIO Support

This `s3-broker` now supports MinIO as well. [MinIO](https://min.io) is an S3 compatible, open source, distributed object system.

## Overview

MinIO integration closely follows AWS provisioning with a few caveats.

1. Each `s3-broker` deployment should have an admin user key scoped to the buckets, policies, and users that will be provisioned.
2. Each user is assumed to be associated to one bucket. That user/bucket combination has a dedicated policy providing access to that one bucket.
3. Each access key is provisioned to one bucket. The key has an expiration, that has an expiration of `0001-01-01 00:00:00` (forever?). The provisioning requires the `s3-broker` to connect with the bucket's user. Since the password cannot be retrieved (and the broker does not store the passwords), the _password is reset_ for every bind operation. Do not rely on the bucket user's password.

## Setup

A sample broker admin policy is in `minio-admin.json`.

This will setup an `s3-broker` user access key with somewhat limited scope:

```
$ mc admin policy add <alias> s3-broker-policy minio-admin-policy.json
Added policy `s3-broker-policy` successfully.
$ mc admin user add <alias> s3-broker
Enter Secret Key: ************
Added user `s3-broker` successfully.
$ mc admin policy set <alias> s3-broker-policy user=s3-broker
Policy s3-broker-policy is set on user `s3-broker`
$ mc admin user list <alias>
enabled    s3-broker             s3-broker-policy
```

If you wish to use the broker user as an alias in the `mc` command:

```
mc --insecure alias set s3broker https://s3.gdc.lan s3-broker
Enter Secret Key: 
Added `s3broker` successfully.
```

References:
* See https://github.com/minio/minio/tree/master/docs/multi-user/admin

## Configuration

The configuration remains nearly identical between AWS and MinIO. However, as MinIO will be a custom location, 
you'll need to add the endpoint in to the configuration. See `config-minio-sample.yml` for a full example.

The new snippet looks like this:
```
s3_config:
  region: us-east-1                 # User the default us-east-1 region
  endpoint: my.minio.server.com     # Fill in your local DNS or IP for the server
  insecure_skip_verify: true        # Set to true if you are using self-signed certificates
  provider: minio                   # Toggle the provider to minio (default is aws)
```

## Samples

To exercise the broker interface without writing code, you can use [Eden](https://starkandwayne.com/blog/welcome-to-eden-a-cli-for-every-open-service-broker-api/) which can be found [here](https://github.com/Qarik-Group/eden).

Setup with environment variables:
```
$ export SB_BROKER_PASSWORD=password
$ export SB_BROKER_USERNAME=username
$ export SB_BROKER_URL=http://127.0.0.1:3000
```

Show the catalog:
```
$ eden catalog
Service   Plan     Free  Description
=======   ====     ====  ===========
minio-s3  default  free  Provides a single S3 bucket with unlimited storage.
~         public   free  Provides a single publicly accessible S3 bucket with unlimited storage.
```

Provising a service (creates a bucket):
```
$ eden provision -s minio-s3 -p default
provision:   minio-s3/default - name: minio-s3-default-6229d92f-346a-4ea0-99dc-3a6642f32090
provision:   done
```

Bind to the service (creates a user):
```
$ eden bind -i 6229d92f-346a-4ea0-99dc-3a6642f32090
Success

Run 'eden credentials -i minio-s3-default-6229d92f-346a-4ea0-99dc-3a6642f32090 -b minio-s3-e32a729b-59f5-4842-a836-758f3e1a3df2' to see credentials
```

Show the credentials:
```
$ eden credentials -i minio-s3-default-6229d92f-346a-4ea0-99dc-3a6642f32090 -b minio-s3-e32a729b-59f5-4842-a836-758f3e1a3df2
{
  "access_key_id": "an-access-key",
  "additional_buckets": [],
  "bucket": "s3bucket-6229d92f-346a-4ea0-99dc-3a6642f32090",
  "endpoint": "s3.gdc.lan",
  "fips_endpoint": "s3-fips.us-east-1.amazonaws.com",
  "insecure_skip_verify": true,
  "region": "us-east-1",
  "secret_access_key": "sekret",
  "uri": "s3://an-access-key:sekret@s3.gdc.lan/s3bucket-6229d92f-346a-4ea0-99dc-3a6642f32090"
}
```

Eden stores the information locally and this can be seen with the services subcommand:
```
$ eden services
Name                                                   Service   Plan     Binding                                        Broker URL
====                                                   =======   ====     =======                                        ==========
minio-s3-default-6229d92f-346a-4ea0-99dc-3a6642f32090  minio-s3  default  minio-s3-e32a729b-59f5-4842-a836-758f3e1a3df2  http://127.0.0.1:3000
```

To remove the binding (deletes a user) - be careful and use the binding _ID_ and not the name:
```
$ eden unbind -i 6229d92f-346a-4ea0-99dc-3a6642f32090 -b e32a729b-59f5-4842-a836-758f3e1a3df2 
Success
```

To deprovision the bucket (removes the bucket):
```
$ eden deprovision -i 6229d92f-346a-4ea0-99dc-3a6642f32090
deprovision: minio-s3/default - guid: 6229d92f-346a-4ea0-99dc-3a6642f32090
deprovision: done
```
