```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:                    # User-defined name (or predefined for public clouds)
    type: <cloud-type>             # ec2, azure, gce, openstack, oci, vsphere, lxd, maas, manual
    auth-types:                    # Authentication types for this cloud
      - <auth-type>                # See cloud-specific docs for valid types
    endpoint: <endpoint>           # API endpoint URL (required for most clouds)
    regions:                       # Optional: define regions/availability zones
      <region-name>:
        endpoint: <endpoint>       # Region-specific endpoint (if different)
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See model configuration docs
    ca-certificates:               # Optional: for private/self-signed clouds
      - <base64-cert>              # Base64-encoded x.509 certificates
```
