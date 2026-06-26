```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all Kubernetes clouds, credentials are stored in `credentials.yaml` on the client and follow this schema.

```{tip}
If using the Juju CLI, you can skip writing this file manually -- `juju add-k8s` can read `kubeconfig` and create the matching credential entry for the selected context.
```

(kubernetes-credential-definition)=

```yaml
credentials:
  <cloud-name>:
    <credential-name>:
      auth-type: <auth-type>          # clientcertificate | oauth2 | userpass
      <auth-attributes>               # fill using one of the mappings below
```

(kubernetes-supported-authentication-types)=
### Authentication types

As for all Kubernetes clouds, the supported authentication types are:

(kubernetes-auth-clientcertificate)=
#### `clientcertificate`

Kubernetes client certificate and key.

- `ClientCertificateData`: The Kubernetes certificate data (required).
- `ClientKeyData`: The Kubernetes certificate key (required).
- `rbac-id`: The unique ID key name of the RBAC resources (optional).

(kubernetes-auth-oauth2)=
#### `oauth2`

OAuth2 token authentication.

- `Token`: The Kubernetes token (required).
- `rbac-id`: The unique ID key name of the RBAC resources (optional).

(kubernetes-auth-userpass)=
#### `userpass`

Username and password authentication.

- `username`: The username to authenticate with (required).
- `password`: The password for the specified username (required).

(kubernetes-auth-certificate)=
#### `certificate` (legacy)

Kubernetes service account token with certificate.

- `ClientCertificateData`: The Kubernetes certificate data (required).
- `Token`: The Kubernetes service account bearer token (required).
- `rbac-id`: The unique ID key name of the RBAC resources (optional).

(kubernetes-auth-oauth2withcert)=
#### `oauth2withcert` (legacy)

OAuth2 token with certificate.

- `ClientCertificateData`: The Kubernetes certificate data (required).
- `ClientKeyData`: The Kubernetes private key data (required).
- `Token`: The Kubernetes token (required).
