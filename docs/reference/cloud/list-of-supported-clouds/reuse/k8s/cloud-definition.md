```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Juju | Add a Kubernetes cloud <add-a-kubernetes-cloud>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all Kubernetes clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client and following this schema.

```{tip}
If using the Juju CLI, you can skip writing this file manually -- `juju add-k8s` can read `kubeconfig` and create the cloud definition for you.
```

```yaml
clouds:
  <cloud-name>:                    # User-defined name for the cluster
    type: kubernetes               # Always 'kubernetes' for Kubernetes clouds
    auth-types:                    # Authentication types
      - clientcertificate          # or: oauth2, userpass (legacy compatibility only: certificate, oauth2withcert)
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
