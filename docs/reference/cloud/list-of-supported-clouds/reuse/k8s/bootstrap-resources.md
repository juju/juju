```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

As for all Kubernetes clouds, bootstrapping a controller creates the following resources in the cluster.

(kubernetes-bootstrap-behavior)=
When bootstrapping a controller on a Kubernetes cloud, Juju creates a namespace for the controller and deploys the controller as a `StatefulSet` with associated resources. The controller manages the Juju state database (MongoDB) and API server within Kubernetes pods.

(kubernetes-resources-created-at-bootstrap)=
Resources created at bootstrap:

- **`Namespace`**: A dedicated namespace for the controller (named `controller-<controller-name>`).
- **`Service`**: A Kubernetes `Service` to expose the controller API (type depends on the cloud: `LoadBalancer` for public clouds, `ClusterIP` for localhost clouds).
- **`ServiceAccount`**: A service account for the controller with cluster-admin privileges.
- **`ClusterRoleBinding`**: Binds the controller service account to the cluster-admin `ClusterRole`.
- **`StatefulSet`**: A `StatefulSet` with the controller pod containing two containers: `mongodb` (Juju's state database) and `api-server` (Juju API server).
- **`Secret`s**: Multiple secrets for TLS certificates (`server.pem`), shared secrets, and optionally docker registry credentials for private image registries.
- **`ConfigMap`s**: Configuration maps for bootstrap parameters and agent configuration.
- **`PersistentVolume`** and **`PersistentVolumeClaim`**: Storage for the controller's operator-storage (MongoDB data and API server state).
- **Proxy resources** (if using `ClusterIP` `Service`): Additional `ConfigMap`, `Role`, `RoleBinding`, and `ServiceAccount` for cluster IP proxy access.
