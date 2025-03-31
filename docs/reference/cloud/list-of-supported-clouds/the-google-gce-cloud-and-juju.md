(cloud-gce)=
# The Google GCE cloud and Juju


This document describes details specific to using your existing Google GCE cloud with Juju. 

> See more: [Google GCE](https://cloud.google.com/compute) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements:

Permissions: Service Account Key Admin, Compute Instance Admin, and Compute Security Admin. <br> See more: [Google \| Compute Engine IAM roles and permissions](https://cloud.google.com/compute/docs/access/iam).


## Notes on `juju add-cloud`

Type in Juju: `gce`

Name in Juju: `google`

## Notes on `juju add-credential`


### Authentication types

#### `oauth2`
Attributes:
- client-id: client ID (required)
- client-email: client e-mail address (required)
- private-key: client secret (required)
- project-id: project ID (required)

#### `jsonfile`
Attributes:
- file: path to the .json file containing a service account key for your project
Path (required)


### If you want to use environment variables:

`CLOUDSDK_COMPUTE_REGION` <p> - `GOOGLE_APPLICATION_CREDENTIALS=<link to JSON credentials file>`

<!--
## Notes on `juju bootstrap`
-->

## Cloud-specific model configuration keys


### base-image-path
Base path to look for machine disk images.

|               |               |
|---------------|---------------|
| type          | string        |
| default value | schema.omit{} |
| immutable     | false         |
| mandatory     | false         |



## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                     |
|----------------------------------------|-----------------------------------------------------|
| conflicting:                           | `instance-type` vs. `[arch, cores, cpu-power, mem]` |
| supported?                             |                                                     |
| - {ref}`constraint-allocate-public-ip` | &#10003;                                            |
| - {ref}`constraint-arch`               | &#10003;                                            |
| - {ref}`constraint-container`          | &#10003;                                            |
| - {ref}`constraint-cores`              | &#10003;                                            |
| - {ref}`constraint-cpu-power`          | &#10003;                                            |
| - {ref}`constraint-image-id`           | &#10005;                                            |
| - {ref}`constraint-instance-role`      | &#10005;                                            |
| - {ref}`constraint-instance-type`      | &#10003;                                            |
| - {ref}`constraint-mem`                | &#10003;                                            |
| - {ref}`constraint-root-disk`          | &#10003;                                            |
| - {ref}`constraint-root-disk-source`   | &#10005;                                            |
| - {ref}`constraint-spaces`             | &#10005;                                            |
| - {ref}`constraint-tags`               | &#10005;                                            |
| - {ref}`constraint-virt-type`          | &#10005;                                            |
| - {ref}`constraint-zones`              | &#10003;                                            |

## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| {ref}`placement-directive-machine`               | TBA      |
| {ref}`placement-directive-subnet`                | &#10005; |
| {ref}`placement-directive-system-id`             | &#10005; |
| {ref}`placement-directive-zone`                  | &#10003; |

