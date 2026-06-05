---
myst:
  html_meta:
    description: "Learn how Kubernetes clouds work with Juju, including concept mappings, authentication types, and cloud configuration requirements."
---

(kubernetes-cloud)=
# Kubernetes cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

On Kubernetes clouds, Juju deploys charms as pods, services, and other Kubernetes resources into an existing Kubernetes cluster. Unlike {ref}`machine clouds <machine-cloud>`, Juju does not provision the cluster infrastructure itself -- it manages application workloads on top of an already running Kubernetes cluster.

See {ref}`list-of-supported-kubernetes-clouds` for a list of Kubernetes distributions that Juju supports.

## Cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

```{note}
On Kubernetes clouds, both the cloud definition and the credentials are typically added through `juju add-k8s`, which reads from your kubeconfig file. This is easier than manually creating cloud definition and credential files.
```

(kubernetes-requirements)=
### Requirements

- A running Kubernetes cluster (any conformant distribution: EKS, GKE, AKS, MicroK8s, Canonical Kubernetes, etc.)
- kubectl configured with cluster access
- Sufficient RBAC permissions to create namespaces, deployments, services, and other resources

(kubernetes-definition)=
### Definition

A Kubernetes cloud in Juju represents an existing Kubernetes cluster. Juju connects to the cluster via the Kubernetes API and manages application deployments within namespaces.

(kubernetes-cloud-definition-file)=
### Cloud definition file

If you prefer to add a Kubernetes cloud from a YAML file rather than using `juju add-k8s`, use the following template:

```yaml
clouds:
  <cloud-name>:                    # User-defined name for the cluster
    type: kubernetes               # Always 'kubernetes' for Kubernetes clouds
    auth-types:                    # Authentication types
      - certificate                # or: clientcertificate, oauth2, oauth2withcert, userpass
    endpoint: <endpoint>           # Kubernetes API server URL
    host-cloud-region: <cloud>/<region>  # Optional: host cloud for the cluster (e.g., ec2/us-west-2)
    regions:                       # Optional: define regions
      <region-name>:
        endpoint: <endpoint>       # Region-specific endpoint (if different)
    config:                        # Optional: model config defaults
      operator-storage: <class>    # Storage class for operator storage
      workload-storage: <class>    # Storage class for workload storage
    ca-certificates:               # Optional: cluster CA certificates
      - <base64-cert>              # Base64-encoded x.509 certificates
```

```{ibnote}
See more: {ref}`manage-clouds`, {ref}`add-a-kubernetes-cloud`
```

(kubernetes-concept-mapping)=
### Kubernetes-to-Juju concept mapping

If you are familiar with Kubernetes, the following maps Kubernetes concepts to their Juju equivalents:

| Kubernetes | Juju |
| - | - |
| [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) | {ref}`model <model>` |
| [node](https://kubernetes.io/docs/concepts/architecture/nodes/) | {ref}`machine <machine>` (on Kubernetes clouds, not managed by Juju) |
| [pod](https://kubernetes.io/docs/concepts/workloads/pods/) | {ref}`unit <unit>` |
| container | process in a unit |
| [service](https://kubernetes.io/docs/concepts/services-networking/service/) | {ref}`application <application>` |

(kubernetes-credential)=
## Credentials

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(kubernetes-supported-authentication-types)=
### Supported authentication types

Kubernetes clouds support the following authentication types:

(kubernetes-auth-certificate)=
- **`certificate`**: Kubernetes service account token with certificate.
  - ClientCertificateData: The kubernetes certificate data (required).
  - Token: The kubernetes service account bearer token (required).
  - rbac-id: The unique ID key name of the rbac resources (optional).

(kubernetes-auth-clientcertificate)=
- **`clientcertificate`**: Kubernetes client certificate and key.
  - ClientCertificateData: The kubernetes certificate data (required).
  - ClientKeyData: The kubernetes certificate key (required).
  - rbac-id: The unique ID key name of the rbac resources (optional).

(kubernetes-auth-oauth2)=
- **`oauth2`**: OAuth2 token authentication.
  - Token: The kubernetes token (required).
  - rbac-id: The unique ID key name of the rbac resources (optional).

(kubernetes-auth-oauth2withcert)=
- **`oauth2withcert`**: OAuth2 token with certificate.
  - ClientCertificateData: The kubernetes certificate data (required).
  - ClientKeyData: The kubernetes private key data (required).
  - Token: The kubernetes token (required).

(kubernetes-auth-userpass)=
- **`userpass`**: Username and password authentication.
  - username: The username to authenticate with (required).
  - password: The password for the specified username (required).

(kubernetes-controller)=
## Controllers

```{ibnote}
See also: {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(kubernetes-bootstrap-behavior)=
### Bootstrap behavior

When bootstrapping a controller on a Kubernetes cloud, Juju creates a namespace for the controller and deploys the controller as a StatefulSet with associated resources. The controller manages the Juju state database (MongoDB) and API server within Kubernetes pods.

(kubernetes-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Namespace**: A dedicated namespace for the controller (named `controller-<controller-name>`).
- **Service**: A Kubernetes Service to expose the controller API (type depends on the cloud: LoadBalancer for public clouds, ClusterIP for localhost clouds).
- **ServiceAccount**: A service account for the controller with cluster-admin privileges.
- **ClusterRoleBinding**: Binds the controller service account to the cluster-admin ClusterRole.
- **StatefulSet**: A StatefulSet with the controller pod containing two containers: `mongodb` (Juju's state database) and `api-server` (Juju API server).
- **Secrets**: Multiple secrets for TLS certificates (`server.pem`), shared secrets, and optionally docker registry credentials for private image registries.
- **ConfigMaps**: Configuration maps for bootstrap parameters and agent configuration.
- **PersistentVolumeClaim**: Storage for the controller's operator-storage (MongoDB data and API server state).
- **Proxy resources** (if using ClusterIP service): Additional ConfigMap, Role, RoleBinding, and ServiceAccount for cluster IP proxy access.

(kubernetes-bootstrap-service-type)=
### Controller service type

When bootstrapping a controller, Juju creates a Kubernetes Service to expose the controller API. The Service type depends on the host cloud platform where the Kubernetes cluster is running:

- **LoadBalancer**: For managed Kubernetes on public clouds
  - Amazon EKS (on EC2)
  - Google GKE (on GCE)
  - Microsoft AKS (on Azure)
  - Charmed Kubernetes on OpenStack
  - Charmed Kubernetes on MAAS (experimental)
- **ClusterIP**: For localhost and development environments
  - MicroK8s
  - Kubernetes on LXD
  - Other/unrecognized host clouds (default)

```{note}
LoadBalancer creates a cloud load balancer with a public IP, while ClusterIP uses internal cluster networking with optional proxy access.
```

(kubernetes-model)=
## Models

```{ibnote}
See also: {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(kubernetes-model-configuration-keys)=
### Model configuration keys

Kubernetes clouds support the following cloud-specific model configuration keys:

(kubernetes-model-config-operator-storage)=
- **`operator-storage`**: The storage class used to provision operator storage. Type: string. Default: "" (uses cluster default storage class). Immutable: true. Mandatory: false.

(kubernetes-model-config-workload-storage)=
- **`workload-storage`**: The preferred storage class used to provision workload storage. Type: string. Default: "" (uses cluster default storage class). Immutable: false. Mandatory: false.

(kubernetes-application)=
## Applications

```{ibnote}
See also: {ref}`Juju | Manage applications <manage-applications>`
```

(kubernetes-supported-constraints)=
### Supported constraints

Kubernetes clouds support the following constraints:

Kubernetes clouds support a limited subset of constraints compared to machine clouds:

- {ref}`constraint-cpu-power`: CPU resource request/limit for pods.
- {ref}`constraint-mem`: Memory resource request/limit for pods.
- {ref}`constraint-tags`: Used for pod affinity and anti-affinity rules.

```{ibnote}
Constraints like `arch`, `cores`, `instance-type`, `root-disk`, `zones`, and others are not supported on Kubernetes clouds. Kubernetes manages node resources and pod scheduling.
```

(kubernetes-placement-directives)=
### Placement directives

Placement directives are not supported on Kubernetes clouds. Pod placement is controlled by Kubernetes scheduling, node selectors, and affinity rules (configured via constraints).

(kubernetes-resources-per-application)=
### Resources created per application

When deploying an application to a Kubernetes model, Juju creates:

- **Deployment, StatefulSet, or DaemonSet**: Depending on the charm specification and application type. StatefulSets are used for applications requiring stable network identities and persistent storage. Deployments are used for stateless applications. DaemonSets run one pod per node.
- **Pod**: One or more pods containing the application's charm containers. Each pod typically includes an init container (`juju-init`) and a main container (`juju-operator`).
- **Service**: A Kubernetes Service to expose the application within the cluster or externally.
- **ConfigMap**: Configuration data for the application.
- **Secret**: Sensitive data like credentials.
- **PersistentVolumeClaim**: If the charm requires storage, one PVC per unit is created based on the configured storage class.

(kubernetes-pod-deployment-patterns)=
### Pod deployment patterns

Kubernetes application pods in Juju follow these patterns:

- **Init container** (`juju-init`): Prepares the pod environment before the main container starts.
- **Operator container** (`juju-operator`): Runs the charm logic and manages the application lifecycle.
- **Workload containers**: Additional containers defined by the charm (e.g., database, web server).

(kubernetes-storage)=
## Storage

```{ibnote}
See also: {ref}`Juju | Manage storage <manage-storage>`
```

### Storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

In addition to {ref}`generic storage providers <storage-provider>`, Kubernetes-based models have access to the following {ref}`cloud-specific storage providers <storage-provider-cloud-specific>`:

(storage-provider-kubernetes)=
#### `kubernetes`

```{ibnote}
See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)
```

The `kubernetes` storage provider provisions storage using Kubernetes PersistentVolumeClaims (PVCs). The underlying storage is provided by the cluster's configured storage classes.

Configuration options:

- **`storage-class`**: The storage class for the Kubernetes cluster to use. It can be any storage class defined in your cluster, for example: `juju-unit-storage`, `juju-charm-storage`, `microk8s-hostpath`, `gp2`, `standard`, etc.

- **`storage-provisioner`**: The Kubernetes storage provisioner. For example: `kubernetes.io/no-provisioner`, `kubernetes.io/aws-ebs`, `kubernetes.io/gce-pd`, `microk8s.io/hostpath`, etc.

- **`parameters.type`**: Extra parameters passed to the storage provisioner. For example: `gp2`, `pd-standard`, etc.
