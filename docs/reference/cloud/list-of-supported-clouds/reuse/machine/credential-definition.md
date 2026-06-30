```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  <cloud-name>:                    # Cloud name from your Juju cloud definitions (for example aws, openstack, my-vsphere)
    <credential-name>:             # User-defined credential name; must be unique within this cloud
      auth-type: <auth-type>       # Cloud-specific auth type (for example access-key, userpass, certificate)
      <attribute>: <value>         # Auth-type-specific field (for example access-key, secret-key, username, password)
      <attribute>: <value>         # Additional required or optional auth fields for the selected auth-type
```
