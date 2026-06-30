```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`; {ref}`storage-provider`
```

### Storage providers

As for all Kubernetes clouds, in addition to the generic storage providers, this cloud also has access to the following {ref}`cloud-specific storage provider <storage-provider-cloud-specific>`:

#### `kubernetes`

```{ibnote}
See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)
```

The `kubernetes` storage provider provisions storage using Kubernetes PersistentVolumeClaims (PVCs). The underlying storage is provided by the cluster's configured storage classes.

Configuration options:

- **`storage-class`**: The storage class for the Kubernetes cluster to use. It can be any storage class defined in your cluster, for example: `microk8s-hostpath`, `gp2`, `standard`, etc.

- **`storage-provisioner`**: The Kubernetes storage provisioner. For example: `kubernetes.io/no-provisioner`, `kubernetes.io/aws-ebs`, `kubernetes.io/gce-pd`, `microk8s.io/hostpath`, etc.

- **`parameters.type`**: Extra parameters passed to the storage provisioner. For example: `gp2`, `pd-standard`, etc.
