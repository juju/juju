As for all Kubernetes clouds, application pods follow these deployment patterns.

```{ibnote}
See also: {ref}`unit`, {ref}`Juju | Manage units <manage-units>`
```

(kubernetes-resources-per-application)=
When deploying an application to a Kubernetes model, Juju creates:

- **`Deployment`, `StatefulSet`, or `DaemonSet`**: Depending on the charm specification and application type. `StatefulSet`s are used for applications requiring stable network identities and persistent storage. `Deployment`s are used for stateless applications. `DaemonSet`s run one pod per node.
- **Pod**: One or more pods containing the application's charm containers. Each pod typically includes an init container (`juju-init`) and a main container (`juju-operator`).
- **`Service`**: A Kubernetes `Service` to expose the application within the cluster or externally.
- **`ConfigMap`**: Configuration data for the application.
- **`Secret`**: Sensitive data like credentials.
- **`PersistentVolume`** and **`PersistentVolumeClaim`**: If the charm requires storage, one PV/PVC per unit is created based on the configured storage class.

(kubernetes-pod-deployment-patterns)=
Kubernetes application pods in Juju follow these patterns:

**Sidecar charms** (current pattern):

- **Init container** (`charm-init`): Prepares the pod environment before the main container starts.
- **Charm container** (`charm`): Runs the charm logic alongside the workload.
- **Workload containers**: Defined by the charm (e.g., database, web server).

**Operator charms** (older pattern):

- **Init container** (`juju-init`): Prepares the pod environment before the main container starts.
- **Operator container** (`juju-operator`): Runs the charm logic and manages the application lifecycle.
- **Workload containers**: Defined by the charm.
