---
myst:
  html_meta:
    description: "Juju secret backend reference: controller registrations, model storage defaults, Vault and Kubernetes settings, revision movement, and backend retirement."
---

(secret-backend)=
# Secret backend

```{ibnote}
See also: {ref}`Juju CLI | Manage secret backends <manage-secret-backends>`, {ref}`Terraform Provider for Juju | Manage secret backends <tfjuju:manage-secret-backends>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Secret backend selection
:alt: Two models on one controller use different registrations: Payments selects myvault, which points to Vault; Ledger uses internal, which points to its model database.
:caption: A secret backend lets a model use a store by name. Here the Payments and Ledger models share a controller but not a choice of store.
```

In Juju, a **secret backend** is a {ref}`controller <controller>` registration that tells Juju where to store the value of a {ref}`secret <secret>` revision. Registering a backend makes it available to the controller's {ref}`models <model>`; it does not select it for any of them. A model's {ref}`selection <secret-backend-active>` determines where new values go. An existing revision can {ref}`remain on its previous backend <secret-backend-movement>` after that selection changes.

(secret-backend-configuration-options)=
## Backend names and types

Each registration has an ID and a name unique to its controller. A model selects a registration by its name. The provider type determines where Juju stores the values: `controller` stores them in Juju's model database, while `vault` and `kubernetes` store them externally. For an external store, Juju keeps the revision and a reference to its value. Both machine and Kubernetes models can select a registered Vault backend.

`internal`, `kubernetes`, `auto` and names ending in `-local` are reserved. They cannot be used to name or rename a registration. The provider settings below configure a connection, not the external service itself. The {ref}`token refresh interval <secret-backend-rotation>` is a property of the registration, not a provider setting.

(secret-backend-vault-settings)=
### Vault backend settings

Juju checks that Vault is reachable, initialized and unsealed, and that it can read key status. An endpoint is the only mandatory schema field; the credentials still need to authorize the operations Juju performs. Choose an HTTPS endpoint and trust settings appropriate for the deployment; follow [Vault's production deployment guidance](https://developer.hashicorp.com/vault/docs/concepts/production-hardening) when provisioning the store.

| Setting | Meaning |
| --- | --- |
| `endpoint` | Vault API URL. Required; cannot be changed after registration. |
| `namespace` | Vault namespace for requests. Optional; cannot be changed after registration. |
| `mount-path` | Optional prefix for the per-model mount. Juju appends the model name, a hyphen and the last six characters of the model UUID. The prefix cannot be changed after registration. |
| `token` | Vault authentication token. |
| `ca-cert` | PEM contents of the CA certificate used to verify Vault, not a local filename. |
| `client-cert`, `client-key` | PEM contents of the TLS client certificate and its matching unencrypted private key. Supply both or neither. |
| `tls-server-name` | Server name to use for TLS. |

(secret-backend-kubernetes-settings)=
### Kubernetes backend settings

For a named Kubernetes backend, Juju checks API discovery, the existing namespace and, if configured, the service account. These checks do not write a Secret. Arrange the credentials and [Kubernetes authorization](https://kubernetes.io/docs/reference/access-authn-authz/authorization/) for the store before registering it.

| Setting | Meaning |
| --- | --- |
| `endpoint`, `namespace` | Kubernetes API URL and the namespace in which to store values. Both are required and cannot change. Registration does not create the namespace. |
| `prefer-incluster-address` | If true, use the in-cluster connection when available; outside a cluster, use the configured connection. Off unless enabled. |
| `ca-certs`, `ca-cert` | List of PEM CA certificates, or a single PEM certificate. The list takes precedence. Supply the trust material the endpoint requires. |
| `client-cert`, `client-key` | PEM contents of the client certificate and matching unencrypted private key. Supply both or neither. |
| `token` | Bearer token for the API. |
| `username`, `password` | Credentials for an API that accepts username/password authentication. |
| `service-account` | Existing account for token refresh. Required when supplying a refresh interval, including zero on update. |
| `skip-tls-verify` | Disables server certificate verification. Do not use this as a substitute for configuring trust in production. |

(secret-backend-built-ins)=
## Built-in backends

At bootstrap Juju supplies two protected registrations. `internal`, of type `controller`, stores values in the model database. `kubernetes`, of type `kubernetes`, uses the model's cluster and namespace (the controller namespace for the controller model). A user-added Kubernetes backend instead uses its own connection settings and namespace. Juju protects the built-ins from update and removal because of their origin, not because of their provider type.

(secret-backend-active)=
## Active backend and model defaults

Each model selects one backend. With the default selection, `auto`, a machine model uses `internal` and a Kubernetes model uses its built-in Kubernetes store, regardless of the controller's own model type. `auto` is a selection, not another registration. New user and charm secret revisions use this active backend; their writers do not select a store per revision.

(secret-backend-encryption-at-rest)=
## Encryption at rest

```{warning}
Whether a revision's value is encrypted at rest depends on the store that holds it. Base64 is an encoding, not encryption. TLS protects the connection to a backend, not the stored value.

- **Controller (`internal`):** Juju writes base64-encoded values to the model database without encrypting them. Any encryption of the underlying storage is separate from Juju.
- **Kubernetes:** Juju writes a Kubernetes Secret. By default, the Kubernetes API server stores Secrets unencrypted in etcd. Check whether the destination cluster [encrypts Secrets at rest](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/); selecting the backend does not enable encryption.
- **Vault:** Juju writes the value to Vault's key/value store. [Vault encrypts it](https://developer.hashicorp.com/vault/docs/internals/architecture) before writing it to persistent storage.
```

If the model's selection is `auto`, see {ref}`secret-backend-active`. If the selection has changed, {ref}`check the revision's recorded backend <check-secret-backend-movement>` and see {ref}`what happens to the previous copy <secret-backend-movement>`.

(secret-backend-register)=
## Registering a secret backend

```{ibnote}
See also: {ref}`Juju CLI | Add a secret backend <add-a-secret-backend>`, [Terraform Provider for Juju | Add a secret backend](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-secret-backends/#add-a-secret-backend)
```

The controller validates the provider settings and checks the proposed store before recording the registration. Unless the client supplies an ID, the controller allocates a UUID. Reusing a mutable registration's ID can update its name and rotation interval instead of creating a second backend; it does not change its type. {ref}`Preparing a model migration <migrate-a-model>` does not normally require copying an ID.

**Who can do this:** A controller {ref}`superuser <user-access-controller-superuser>`.

**Errors:**

- `secret backend "…" already exists`: another registration has the name. Inspect it before choosing a different name.
- `reserved name "…"`: choose a nonreserved {ref}`backend name <secret-backend-configuration-options>`.
- `config for provider` identifies invalid settings. Correct the named provider's configuration. If the connection check fails instead, correct connectivity, credentials or provider state before retrying.
- `permission denied`: ask a superuser on the controller where the backend is needed to register it.

(secret-backend-inspection)=
## Listing and inspecting secret backends

```{ibnote}
See also: {ref}`Juju CLI | List secret backends <list-secret-backends>`, {ref}`Juju CLI | View details about a secret backend <inspect-a-secret-backend>`
```

The controller inventory includes unused named registrations. For the built-in Kubernetes store it lists `<model>-local` where revisions reference that store, not one `kubernetes` row for every model. Looking up an exact name filters this same inventory. No match yields an empty result, not proof that the controller has no backends.

The secret count is the number of **distinct secrets** with revisions referencing the backend; a model-local Kubernetes row counts that model's secrets. A single secret can count on two backends while its revisions are on different stores. The count is not the number of revisions, models using the backend, or objects present in the provider.

An inspection probes Vault. A failed check returns status `error` and a message, such as `vault is sealed`. The controller does not probe a Kubernetes or controller backend on this read: `active` is neither a test write nor an indication that a model selected the registration.

**Who can do this:** An authenticated controller user. Only a controller {ref}`superuser <user-access-controller-superuser>` can request sensitive configuration fields. Even that result contains backend settings, not the secret values in the store.

**Errors:** `permission denied` on a revealed read: inspect without requesting sensitive fields, or use a superuser account authorized to see them.

(secret-backend-check-selection)=
## Checking a model's selected backend

```{ibnote}
See also: {ref}`Juju CLI | Check a model's selected backend <check-model-secret-backend>`
```

The model reports the registered name or `auto` for its platform default. This is the choice for future writes. It does not list revision locations or tell the reader whether a move has finished.

**Who can do this:** A user with at least model {ref}`read <user-access-model-read>` access to that model.

**Errors:** `permission denied`: check the target model and ask its administrator for read access.

(secret-backend-select)=
## Selecting a backend for a model

```{ibnote}
See also: {ref}`Juju CLI | Select a backend for a model <check-model-secret-backend>`, [Terraform Provider for Juju | Set the secret backend for a model](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-secret-backends/#set-the-secret-backend-for-a-model)
```

A model writer can select an existing registration by name, or restore the platform default with `auto`. Success changes only that model's selection. The controller does not check provider health or wait for any values to move before accepting the selection.

**Who can do this:** A user with at least model {ref}`write <user-access-model-write>` access to that model.

**Errors:**

- `secret backend "…" not found`: check the name and controller. If necessary, ask a controller superuser to register the store first.
- `secret backend "…" not valid`: `internal`, `kubernetes` and model-local display names cannot be selected explicitly. Use `auto` for the default.
- `permission denied`: model read access is not enough to change the selection; ask a model writer.

(secret-backend-movement)=
## Moving revisions automatically after a backend change

```{ibnote}
See also: {ref}`Juju CLI | Check where existing secret revisions are stored <check-secret-backend-movement>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Moving a secret revision
:alt: After the controller records the selection, it can reply to the client while a worker independently reads an old revision, saves its value to the new store, updates the reference and attempts cleanup.
:caption: Moving a revision between two external stores changes its recorded location, not its revision number. The reply acknowledges the selection; it does not acknowledge any step of the move.
```

Juju workers move eligible revisions in the background. If a worker records a new external location but cannot clean up the old copy, the new location can appear in Juju while the old value still exists in the provider. Neither the selection nor the {ref}`revision metadata <secret-revision-backend>` guarantees a completion time or remote erasure.

**Who can do this:** Juju moves revisions automatically. There is no separate move command for the operator to run.

(secret-backend-update)=
## Updating a secret backend

```{ibnote}
See also: {ref}`Juju CLI | Update a secret backend <update-a-secret-backend>`, [Terraform Provider for Juju | Update a secret backend](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-secret-backends/#update-a-secret-backend)
```

An update identifies an existing registration by its current name. Juju overlays supplied settings on the existing configuration, then removes keys requested for reset, restoring provider defaults where defined. Juju validates the result and normally checks the connection. Skipping that check does not bypass validation, built-in protection, access checks or name conflicts.

A rename preserves the registration's ID, type and model selections. To connect to a different immutable endpoint, register a separate backend and select it for the intended models instead.

**Who can do this:** A controller {ref}`superuser <user-access-controller-superuser>`.

**Errors:**

- `secret backend "…" not found`: use the current name on the correct controller.
- `cannot rename secret backend` with a name conflict: choose a free, nonreserved name.
- `cannot change immutable field`: keep the existing value, including on reset, or register a different backend. A {ref}`built-in registration <secret-backend-built-ins>` cannot itself be updated.
- `permission denied`: ask a superuser on the registration's controller to make the change.

(secret-backend-rotation)=
## Rotating a backend token

```{ibnote}
See also: {ref}`Juju CLI | Configure token rotation <update-a-secret-backend>`
```

Vault and Kubernetes support refreshing the credential Juju uses to access the backend. This does not rotate the value of any stored secret. A positive interval requests a token lifetime of at least one hour. Juju initially schedules a refresh attempt at three quarters of that interval, or after 24 hours, whichever is earlier. On success Juju saves the replacement credentials; a configured interval is not evidence that a refresh succeeded.

An update that omits the interval keeps the current setting. An explicit zero disables refresh. Kubernetes still requires the {ref}`service account <secret-backend-kubernetes-settings>` when an interval is supplied.

**Who can do this:** A controller {ref}`superuser <user-access-controller-superuser>` configures the interval; Juju attempts the refresh.

**Errors:** `less than 1h` means the requested positive interval is too short. Supply at least one hour. A provider that does not support token refresh cannot accept a positive interval.

(secret-backend-remove)=
## Removing a secret backend

```{ibnote}
See also: {ref}`Juju CLI | Remove a secret backend <remove-a-secret-backend>`, [Terraform Provider for Juju | Remove a secret backend](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-secret-backends/#remove-a-secret-backend)
```

Ordinary removal refuses a registration while any revision on the controller references it. A model selecting an otherwise empty backend does not block removal. On success Juju removes the registration, its configuration and its token refresh schedule. **All** models selecting it revert to their own platform defaults.

Forced removal skips the revision-reference check, not {ref}`built-in protection <secret-backend-built-ins>`. It discards tracking references without moving or deleting remote values. User and charm revisions still dependent on the registration can become unreadable through Juju.

**Who can do this:** A controller {ref}`superuser <user-access-controller-superuser>`.

**Errors:**

- `deleting in use secret backend not supported`: leave a built-in in place. For a user-added registration, {ref}`check where revisions are stored <check-secret-backend-movement>` and retry ordinary removal after references are gone.
- `secret backend "…" not found`: inspect the controller inventory for the current name.
- `permission denied`: ask a superuser on that controller to retire the registration.
