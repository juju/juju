```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`; {ref}`storage-provider`
```

### Storage providers

Kubernetes models support filesystem storage. The `kubernetes` provider uses persistent volumes; `rootfs` and `tmpfs` use pod [`emptyDir` volumes](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir).

#### `kubernetes`

```{ibnote}
See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)
```

The `kubernetes` provider uses a [PersistentVolumeClaim (PVC)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims) to request storage from a cluster storage class. The [PersistentVolume (PV)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistent-volumes) supplies the storage.

For {ref}`filesystem import <import-a-filesystem-as-storage>`, the provider takes the PV name. Normal import requires the `Retain` reclaim policy and a PV with no claim reference. Forced import can first change the policy to `Retain`. Before deleting an existing PVC, Juju checks its ownership and binding. If import fails, inspect both objects: The policy may have changed or the old claim may have been removed.

On detachment, Juju sets a bound PV's reclaim policy to `Retain` before deleting the PVC. Destroying the filesystem requests PV deletion; releasing the filesystem leaves the PV in place. To establish whether the data still exists, check the PV and the underlying storage system.

Configuration options:

- **`storage-class`**: The storage class for the Kubernetes cluster to use. It can be any storage class defined in your cluster, for example: `microk8s-hostpath`, `gp2`, `standard`, etc.

- **`storage-provisioner`**: The Kubernetes storage provisioner. For example: `kubernetes.io/no-provisioner`, `kubernetes.io/aws-ebs`, `kubernetes.io/gce-pd`, `microk8s.io/hostpath`, etc.

- **`parameters.type`**: Extra parameters passed to the storage provisioner. For example: `gp2`, `pd-standard`, etc.
