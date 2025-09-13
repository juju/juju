(cloud-gce)=
# The Google GCE cloud and Juju

This document describes details specific to using your existing Google GCE cloud with Juju.

```{ibnote}
See more: [Google GCE](https://cloud.google.com/compute)
```

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

```{ibnote}
See more: {ref}`cloud-differences`
```

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
- `client-id`: client ID (required)
- `client-email`: client e-mail address (required)
- `private-key`: client secret (required)
- `project-id`: project ID (required)

#### `jsonfile`
Attributes:
- `file`: path to the `.json` file containing a service account key for your project
Path (required)

```{ibnote}
See more:
- {ref}`gce-appendix-workflow-2`
```

### If you want to use environment variables:

`CLOUDSDK_COMPUTE_REGION` <p> - `GOOGLE_APPLICATION_CREDENTIALS=<link to JSON credentials file>`

#### `service-account`
> *Requirements:*
> - Juju 3.6+.
> - A service account with sufficient privileges. See more: {ref}`gce-appendix-service-account`
> - The `add-credential` steps must be run from a jump host running in Google Cloud in order to allow the cloud metadata endpoint to be reached.

```{ibnote}
See more: {ref}`gce-appendix-workflow-1`
```

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

### vpc-id-force
Force Juju to use the GCE VPC ID specified with vpc-id, when it fails the minimum validation criteria.

| | |
|-|-|
| type | bool |
| default value | false |
| immutable | true |
| mandatory | false |

### vpc-id
Use a specific VPC network (optional). When not specified, Juju requires a default VPC to be available for the account.

Example: vpc-a1b2c3d4
| | |
|-|-|
| type | string |
| default value | "" |
| immutable | true |
| mandatory | false |



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

## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

(storage-provider-gce)=
### `gce`

Configuration options:

- `disk-type`. Value is `pd-ssd`. Warning: [bug](https://github.com/juju/juju/issues/20349).

(gce-appendix-example-authentication-workflows)=
## Appendix: Example authentication workflows

(gce-appendix-workflow-1)=
### Workflow 1 -- Service account only (recommended)
> *Requirements:*
> - Juju 3.6+.
> - A service account with sufficient privileges. See more: {ref}`gce-appendix-service-account`
> - The `add-credential` steps must be run from a jump host running in Google Cloud in order to allow the cloud metadata endpoint to be reached.

1. Run `juju add-credential google`; choose `service-account`; supply the service account email.
2. Bootstrap as usual.

```{tip}
**Did you know?** With this workflow where you provide the service account during `add-credential` you avoid the need for either your Juju client or your Juju controller to store your credential secrets. Relatedly, the user running `add-credential` / `bootstrap` doesn't need to have any credential secrets supplied to them.
```
```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non controller) machines in the model.
```

(gce-appendix-workflow-2)=
### Workflow 2 -- Bootstrap using normal credential; use service account thereafter
> *Requirements:*
> - Juju 3.6+.
> - A service account with sufficient privileges. See more: {ref}`gce-appendix-service-account`

1. Bootstrap with the arg `--bootstrap-constraints="instance-role=auto"`
2. The controller machines will be created and attached to the project's default service account. 
3. Alternatively you can specify a different service account instead of `auto`.

```{tip}
To configure workload machines to use a different (less privileged) service account, use the `instance-role` constraint. This can be set on the model to apply to all (non controller) machines in the model.
```

(gce-appendix-service-account)=
## Appendix: Service account requirements

To enlist a service account to provide the privileges required by Juju, the following scopes must be assigned:
- `https://www.googleapis.com/auth/compute`
- `https://www.googleapis.com/auth/devstorage.full_control`
