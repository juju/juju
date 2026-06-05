---
myst:
  html_meta:
    description: "Learn how Kubernetes clouds work with Juju, including concept mappings, authentication types, and cloud configuration requirements."
---

(kubernetes-clouds)=
# Kubernetes clouds

```{toctree}
:hidden:

Amazon EKS <amazon-eks>
Canonical Kubernetes <canonical-kubernetes>
Google GKE <google-gke>
Microsoft AKS <microsoft-aks>
MicroK8s <microk8s>
```

```{ibnote}
See also: {ref}`list-of-supported-kubernetes-clouds`
```

In Juju, a Kubernetes cloud is a {ref}`kubernetes-cloud`. Juju deploys charms as pods, services, and other Kubernetes resources into an existing Kubernetes cluster. Unlike {ref}`machine clouds <machine-cloud>`, Juju does not provision the cluster infrastructure itself -- it manages application workloads on top of an already running Kubernetes cluster.

(kubernetes-cloud)=
## Cloud

(kubernetes-definition)=
### Definition

A Kubernetes cloud in Juju represents an existing Kubernetes cluster. Juju connects to the cluster via the Kubernetes API and manages application deployments within namespaces.

(kubernetes-requirements)=
### Requirements

- A running Kubernetes cluster (any conformant distribution: EKS, GKE, AKS, MicroK8s, Canonical Kubernetes, etc.)
- kubectl configured with cluster access
- Sufficient RBAC permissions to create namespaces, deployments, services, and other resources

(kubernetes-concept-mapping)=
### Juju-to-Kubernetes concept mapping

If you are familiar with Kubernetes, the following maps Juju concepts to their Kubernetes equivalents:

| Juju | Kubernetes |
| - | - |
| {ref}`model <model>` | [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) |
| {ref}`machine <machine>` (not managed by Juju) | [node](https://kubernetes.io/docs/concepts/architecture/nodes/) |
| {ref}`unit <unit>` | [pod](https://kubernetes.io/docs/concepts/workloads/pods/) |
| process in a unit | container |
| {ref}`application <application>` | [service](https://kubernetes.io/docs/concepts/services-networking/service/) |

(kubernetes-credential)=
## Credential

On Kubernetes clouds, both the cloud definition and the credentials are added through `juju add-k8s`, which reads from your kubeconfig file.

(kubernetes-supported-authentication-types)=
### Supported authentication types

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
## Controller

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
### Service type by cloud

The controller Service type varies by cloud:

- **Public clouds** (EKS, GKE, AKS, OpenStack): `LoadBalancer` -- creates a cloud load balancer with a public IP.
- **Localhost clouds** (MicroK8s, LXD): `ClusterIP` -- uses internal cluster networking with optional proxy.
- **MAAS**: `LoadBalancer` (experimental).
- **Other**: `ClusterIP` (default).

(kubernetes-model)=
## Model

(kubernetes-model-configuration-keys)=
### Model configuration keys

(kubernetes-model-config-operator-storage)=
- **`operator-storage`**: The storage class used to provision operator storage. Type: string. Default: "" (uses cluster default storage class). Immutable: true. Mandatory: false.

(kubernetes-model-config-workload-storage)=
- **`workload-storage`**: The preferred storage class used to provision workload storage. Type: string. Default: "" (uses cluster default storage class). Immutable: false. Mandatory: false.

(kubernetes-application)=
## Application

(kubernetes-supported-constraints)=
### Supported constraints

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
## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

Kubernetes-based models have access to the `kubernetes` storage provider.

(storage-provider-kubernetes)=
### `kubernetes`

```{ibnote}
See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)
```

The `kubernetes` storage provider provisions storage using Kubernetes PersistentVolumeClaims (PVCs). The underlying storage is provided by the cluster's configured storage classes.

Configuration options:

- **`storage-class`**: The storage class for the Kubernetes cluster to use. It can be any storage class defined in your cluster, for example: `juju-unit-storage`, `juju-charm-storage`, `microk8s-hostpath`, `gp2`, `standard`, etc.

- **`storage-provisioner`**: The Kubernetes storage provisioner. For example: `kubernetes.io/no-provisioner`, `kubernetes.io/aws-ebs`, `kubernetes.io/gce-pd`, `microk8s.io/hostpath`, etc.

- **`parameters.type`**: Extra parameters passed to the storage provisioner. For example: `gp2`, `pd-standard`, etc.
