(kubernetes-clouds-and-juju)=
# Kubernetes clouds and Juju

In Juju, all Kubernetes clouds behave fundamentally the same.

## Kubernetes in Juju

Juju supports both traditional machine clouds as well as Kubernetes clouds. If you are familiar with Kubernetes, the following is a mapping between Kubernetes and Juju concepts:

| Juju | Kubernetes |
| - | - |
| {ref}`model <model>` | [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) |
| {ref}`machine <machine>`; Juju does not manage this for Kubernetes | [node](https://kubernetes.io/docs/concepts/architecture/nodes/) |
| {ref}`unit <unit>` | [pod](https://kubernetes.io/docs/concepts/workloads/pods/) |
| process in a unit | container |
| {ref}`application <application>` | [service](https://kubernetes.io/docs/concepts/services-networking/service/) |

## Notes on `juju add-k8s`

On Kubernetes clouds, both the cloud definition and the cloud credentials are added through `juju add-k8s`, which reads from your kubeconfig file.

### Authentication types


#### `certificate`
Attributes:
- ClientCertificateData: the kubernetes certificate data (required)
- Token: the kubernetes service account bearer token (required)
- rbac-id: the unique ID key name of the rbac resources (optional)


#### `clientcertificate`
Attributes:
- ClientCertificateData: the kubernetes certificate data (required)
- ClientKeyData: the kubernetes certificate key (required)
- rbac-id: the unique ID key name of the rbac resources (optional)

#### `oauth2`
Attributes:
- Token: the kubernetes token (required)
- rbac-id: the unique ID key name of the rbac resources (optional)

#### `oauth2withcert`
Attributes:
- ClientCertificateData: the kubernetes certificate data (required)
- ClientKeyData: the kubernetes private key data (required)
- Token: the kubernetes token (required)


#### `userpass`
Attributes:
- username: The username to authenticate with. (required)
- password: The password for the specified username. (required)



## Cloud-specific model configuration keys

### `operator-storage`
The storage class used to provision operator storage.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | true |
| mandatory | false |

### `workload-storage`
The preferred storage class used to provision workload storage.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |

## Supported constraints


| {ref}`CONSTRAINT <constraint>`         |                                               |
|----------------------------------------|-----------------------------------------------|
| conflicting:                           | `instance-type` vs. `[cores, cpu-power, mem]` |
| supported?                             |                                               |
| - {ref}`constraint-allocate-public-ip` | &#10005;                                      |
| - {ref}`constraint-arch`               | &#10005;                                      |
| - {ref}`constraint-container`          | &#10005;                                      |
| - {ref}`constraint-cores`              | &#10005;                                      |
| - {ref}`constraint-cpu-power`          | &#10003;                                      |
| - {ref}`constraint-image-id`           | &#10005;                                      |
| - {ref}`constraint-instance-role`      | &#10005;                                      |
| - {ref}`constraint-instance-type`      | &#10005;                                      |
| - {ref}`constraint-mem`                | &#10003;                                      |
| - {ref}`constraint-root-disk`          | &#10005;                                      |
| - {ref}`constraint-root-disk-source`   | &#10005;                                      |
| - {ref}`constraint-spaces`             | &#10005;                                      |
| - {ref}`constraint-tags`               | &#10003; <br> Used for affinity.              |
| - {ref}`constraint-virt-type`          | &#10005;                                      |
| - {ref}`constraint-zones`              | &#10005;                                      |


<!--
Sadly, the mem and cpu-power constraints do not properly do what's needed for requests and limits; what we have is very simplistic.
-->

## Placement directives

Placement directives aren't supported on Kubernetes clouds.
