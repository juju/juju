---
myst:
  html_meta:
    description: "Juju secret reference: the sensitive payload Juju stores on behalf of its owner. The secret record and its revisions, charm and user secrets, access grants, rotation and expiry, backends, and watchers."
---

(secret)=
# Secret

In Juju, a **secret** is a piece of sensitive information (an API key, a
password, a certificate) that Juju stores on behalf of its owner and
shares only with the consumers it is granted to. It is one object across
the three layers: the owner declares it over the controller API or its
unit's hook context, the model database persists its metadata, its owner
record and its revisions, and the owner's unit agent executes it,
turning the policy clocks into events and serving the payload to the
consumers.

(the-secrets-declaration)=
## Secrets in the declaration layer

- **Charm secrets:** The owner unit's hook context carries the secret
  commands: `secret-add`, `secret-set`, `secret-grant`,
  `secret-revoke`, `secret-get`, `secret-info-get`, `secret-ids`,
  `secret-remove`.
- **User secrets:** A Juju client carries the secret commands:
  `add-secret`, `update-secret`, `grant-secret`, `revoke-secret`,
  `remove-secret`; adding, updating, granting and revoking require
  model {ref}`write access <user-access-model-write>`.
- **Backends:** Selecting a model's active backend is its own model
  operation; see {ref}`secret backends <secret-backend>` below.

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`, {ref}`Terraform Provider for Juju | Manage secrets <tfjuju:manage-secrets>`
```

(the-secrets-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - Secret data keys must match the name pattern (lowercase letter
    first, then letters, numbers and dashes, at least three
    characters) and must not be empty.
  - A secret value may be at most 1 MB (1,000,000 bytes)
    base64-encoded per key, and the secret's total content at most
    1 MB.
  - Charm secrets cannot be created with auto-prune; only user secrets
    auto-prune.
  - A rotation policy and a next rotation time always come together;
    one without the other is invalid.
  - A label is unique per owner or consumer scope: an owner's label
    and each consumer's label are unique among that owner's or
    consumer's secrets.
  - The secret ID is minted by Juju; a secret that originated in
    another model carries that model's UUID in its URI.
- **Errors:**
  - **`secret label already exists`:** Triggered when a secret is
    created or updated with a label that its owner or consumer scope
    already uses. Remediation: choose another label.
  - **`charm secrets do not support auto prune`:** Triggered when a
    charm secret is created with auto-prune. Remediation: none;
    auto-prune is a user-secret behavior.
  - **`cannot specify a secret rotate policy without a next rotate
    time`:** Triggered when a rotation policy is set with no next
    rotation time. Remediation: set the policy and the time together.
  - **`cannot specify a secret rotate time without a rotate policy`:**
    Triggered when a next rotation time is set with no rotation
    policy. Remediation: set the policy and the time together.
  - **`key "<key>" not valid`:** Triggered when a secret data key does
    not match the name pattern. Remediation: rename the key with
    lowercase letters, numbers and dashes.

(the-secrets-persistence)=
## Secrets in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Secret attributes
:alt: The secret metadata record at the centre with its salient columns; the revision record and its content record west; the owner and the consumer records east; the permission grants south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory (the owner and consumer labels are nullable fields, not pointers).
:caption: The secret's stored records. A secret is one metadata record keyed by the secret's ID, carrying the latest revision pointer, the description, the rotate policy and the auto-prune flag; the owner and consumer records and the permission grants hang off it, and each revision carries its payload either inline or as a backend reference; every line is a foreign key in one of those records.
```

In the {ref}`model database <data-model-full-spine>`, a secret is a **native record**: the record
set grew to its current shape over the patch stream, and the schema
folds some of its records into the drawn slice. The drawn slice is the
record set every secret carries; the entity's remaining records round
out the specific roles:

- **A secret has one metadata record.** It is keyed by the secret's
  ID, and that ID is the whole identity pair: the owner records, the
  revisions, the consumers and the permission grants all point at
  it. It carries the latest revision pointer, the description, the
  rotation policy, and the auto-prune flag.
- **A secret has an owner** (an application, a unit, or the model;
  one record kind per owner) **and consumers** (the units tracking
  it, in this model and, for cross-model secrets, in the consuming
  model). The owner's and each consumer's label hang there: any
  string, unique per owner or consumer, for the charm's internal
  reference.
- **A secret publishes revisions.** One record per published
  revision, unique per secret and revision number; a revision no
  consumer tracks becomes obsolete, and the expiry time hangs off
  the revision.
- **Each revision carries its payload**: Key/value records when the
  content is stored inline, or a reference into a {ref}`secret
  backend <secret-backend>`'s store (with the deleted-content
  references cleaned up after the external content is).
- **A secret carries permission grants**: A role (none, view,
  manage) over a subject (a unit, an application, the model) within
  a scope (a unit, an application, a model, a relation), one record
  per subject. The roles, the subject kinds and the scope kinds are
  stored vocabularies.
- **The satellites the record set rounds out:** the rotation clock
  (the next rotation time per secret), the ID reservations (IDs
  minted but not yet committed as charm secrets, consumed when the
  secret is created), the cross-model consumer-side record (the
  latest revision and, filled in lazily after a migration, the owner
  application), and the deleted-content references.

The client-facing handles hang off the secret's ID: the **secret
URI**, assigned by Juju when the secret is added and returned to the
caller for subsequent actions; a user-chosen **name** for a user
secret, stored as its owner label; and an **owner** or **consumer**
label for the charm's internal reference. A cross-model secret's URI
carries the source model's UUID as the URI's host part
(`secret://<source-uuid>/<id>`); a local secret's URI is just
`secret:<id>`.

The services read the secret's records through derived views that
join the metadata, policy, revision, expiry and owner records into one
shape, resolve the grant subjects and scopes to natural ids, and
union the owner kinds. The views have no pointers of their own; they
are not drawn.

(the-secret-states)=
One state machine, and it is not the shared life: a secret carries no
alive, dying, dead cycle of the shared kind. The owner publishes
revisions, the controller writes the grant and tracking records, the
rotation and expiry policies fire the owner's hooks, and the owner or
the auto-pruner retires superseded revisions; removing the secret
deletes the records and the backend payloads. The records above store
that story; the operations in the execution layer drive it.

```{ggarch}
:file: ../juju.ggarch
:view: Secret lifecycle
:no-legend:
:caption: The life of a secret, grounded in domain/secret: reserved (URI minted) -> active (latest revision, content in a backend) -> granted (view | manage roles) -> superseded; a rotate policy fires secret-rotate (leader), expiry fires secret-expired; a revision no consumer tracks becomes obsolete (pending delete) and the owner charm retires it via secret-remove (or user secrets auto-prune). Consumers see secret-changed.
:alt: State machine: reserved to active on create, active self-loops for grant/revoke and new-revision publication, active to rotate-due on the rotate policy and back via secret-rotate, active to expiry-due and on to removed via secret-expired then secret-remove, active to obsolete when superseded, obsolete to removed on prune.
```

### Types of secret

A secret is typed by who owns it: the owner record is one of an
application, a unit, or the model, and that is the whole taxonomy. The
type is a stored discriminator: it is which owner record kind the
secret hangs off.

(charm-secret)=
#### Charm secret

```{versionadded} 3.0.0
```

A **charm secret** is a secret created by a charm. A charm secret is
shared with another charm (the secret's observer) over relation data,
and its access is tied to the lifecycle of the relation: a grant
scoped to a relation is deleted when the relation is removed.

(unit-secret)=
##### Unit secret

A **unit secret** is a {ref}`charm secret <charm-secret>` created by a
unit and owned by the unit.

(application-secret)=
##### Application secret

An **application secret** is a {ref}`charm secret <charm-secret>`
created by the leader unit and, because the leader unit does not have
a fixed identity, owned by the application: when the leader unit
changes, the secret is owned by the new leader.

(user-secret)=
#### User secret

```{versionadded} 3.3.0
```

A **user secret** is a secret created by a {ref}`user <user>` with a
{ref}`model admin access level <user-access-model-admin>` and, because
this does not have a fixed identity, owned by the model. A user secret
is shared with a charm (the secret's observer) via a configuration
option. The charm must support the configuration option.

(the-secrets-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - Owner labels and consumer labels are unique within their scope:
    per owner record kind and per consuming unit, enforced by the
    schema's partial unique indexes.
  - Revisions are unique per secret: one record per secret ID and
    revision number.
  - A reserved secret ID is consumed when the secret is committed, and
    a unit can only write backend content for IDs it reserved.
- **Errors:**
  - **`secret not found`:** Triggered when operating on a secret ID
    that does not exist. Remediation: check the URI or the label.
  - **`secret revision not found`:** Triggered when operating on a
    revision that does not exist. Remediation: check the revision
    number.
  - **`secret consumer not found`:** Triggered when the named unit has
    no consumer record for the secret. Remediation: get the secret
    once to start tracking it.
  - **`secret access scope not found`:** Triggered when the grant's
    scope does not exist. Remediation: check the scope entity.
  - **`secret is from a different model`:** Triggered when operating
    locally on a secret whose URI names another model. Remediation:
    consume the secret from the model that owns it.
  - **`missing secret backend id`:** Triggered when importing a secret
    whose backend cannot be identified. Remediation: check the
    backend's configuration.

(the-secrets-execution)=
## Secrets in the execution layer

A secret has machinery of its own: on the controller the secret is
stored, granted and pruned, and on the owner unit's agent the rotation
and expiry workers turn the policy clocks into the `secret-rotate` and
`secret-expired` events. Changing a model's active backend starts a
drain worker that moves the model's secrets to the new backend.

(the-secret-operations)=
### Secret operations

- **Creating and updating:** The owner creates the secret (a charm's
  `secret-add`, a user's `add-secret`): Juju mints the secret ID,
  writes the metadata and owner records, and stores the first
  revision's payload in the active backend. Publishing a new revision
  (a charm's `secret-set`, a user's `update-secret`) adds a revision
  record and notifies the consumers; the consumers then track or peek
  it.
- **Granting and revoking:** Access is a grant record on the secret:
  the owner grants a subject (an application, a unit, or the model) a
  role, view or manage, optionally scoped to a relation, and can
  revoke it. The owner application's units read it implicitly, and the
  relation that carries a grant revokes it when it is removed.
- **Tracking and peeking:** A unit that gets a secret for the first
  time starts tracking its latest revision. A unit cannot choose to
  track an outdated revision, but it can refuse to update to a newer
  one: it can peek the latest revision without updating to it, or do
  nothing.

```{important}

When a unit gets a secret for the first time it will automatically be
set to track the latest revision. A unit cannot choose to track an
outdated revision, but it can in principle refuse to update to a newer
one.

```

- **Rotating:** A rotation policy (hourly, daily, weekly, monthly,
  quarterly, yearly) puts the secret on a rotation clock: when it
  fires, the owner unit's agent runs the charm's `secret-rotate` hook,
  the charm publishes a new revision, and the next rotation time moves
  on. On an application-owned secret the hook runs on the leader unit
  only.
- **Expiring and removing:** An expiry date on a revision fires the
  `secret-expired` event when it passes, telling the owner to retire
  the secret. Revisions no consumer tracks become obsolete: the charm
  removes them (`secret-remove`) or, for secrets with auto-prune, Juju
  deletes them itself. Removing a secret deletes its records and its
  backend payloads.

```{important}

Charms that create secrets should _always_ handle the `secret-remove`
event. That is because secret revisions, even if obsolete, remain
until removed by the charm; if a charm does not remove them, they
accumulate indefinitely.

```

- **User secrets:** The user's workflow: the user creates the secret
  (`add-secret`), grants an application access (`grant-secret`, which
  fires no hook), sets the application's configuration option to the
  secret URI (which fires `config-changed` on the charm), and updates
  the content (`update-secret`, which fires `secret-changed` on the
  observing units). The only hook a user-secret observer receives is
  `secret-changed`: user secrets have no rotate, expire or remove
  lifecycle.

(secret-backend)=
### Secret backends

```{ibnote}
See also: {ref}`manage-secret-backends`
```

A **secret backend** is a service that is used to store sensitive
content which Juju manages as {ref}`secrets <secret>`. The backend
records live in the controller database; each model points at one
active backend.

A secret backend is identified by a name and a type, and admits various
configuration options, some of them generic and some
backend-type-specific.

#### Name

The name of a secret backend can be:

- `auto` (i.e., `internal` for machine models and the model's built-in
  `<model name>-local` backend for Kubernetes models)
- `internal`
- `<some custom name>`

The model's active backend is set per model, as its own model operation
(see {ref}`manage secret backends <manage-secret-backends>`); it is no
longer model configuration: the `secret-backend` model configuration
key is gone.

#### Type

The type of a secret backend can be `controller`, `kubernetes`, and
`vault`.

```{tip}
For production use, we recommend `vault`.
```

##### `controller`

The `controller` backend is the Juju model database.

It is the default secret backend for machine (VM) models.

##### `kubernetes`

The `kubernetes` backend is the model's Kubernetes namespace.

It is the default secret backend for container (Kubernetes) models.

Available starting with Juju 3.1.

##### `vault`

The `vault` backend refers to the Hashicorp Vault.

It is available as an opt-in to both machine and Kubernetes models.

Available starting with Juju 3.1.

(secret-backend-configuration-options)=
#### Configuration options

##### Generic

The generic configuration keys currently include just the following:

|||
|-|-|
|`token-rotate` | The maximum period for which an access token is valid for. Some time prior to the token expiring Juju will generate a new one. Possible values: standard Go duration (e.g., 2h, 7d). The minimum is 1h.|

The `vault` backend is the only one that supports it for now.

##### Backend-specific

The `vault` backend supports the following configuration keys:

|                   |                                                                                                                                                 |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| `ca-cert`         | The path to a PEM-encoded CA certificate file on the local disk. This file is used to verify the Vault server's SSL certificate.                |
| `client-cert`     | The path to a PEM-encoded client certificate on the local disk. This file is used for TLS communication with the Vault server.                  |
| `client-key`      | An unencrypted, PEM-encoded private key on disk which corresponds to the matching client certificate.                                           |
| `endpoint`        |                                                                                                                                                 |
| `namespace`       | The namespace to use for the secret store. Setting this is not necessary but allows using relative paths.                                       |
| `mount-point`     | The mount point to use as a prefix for the secret store. If specified, secrets are stored under `<mount-point>/<model-name>-<model-shortuuid>`. |
| `tls-server-name` | The name to use as the SNI host when connecting via TLS.                                                                                        |
| `token`           | The vault authentication token.                                                                                                                 |

```{ibnote}
See more: [Vault | `vault server`](https://fig.io/manual/vault/server), [Hashicorp | Vault CLI](https://developer.hashicorp.com/vault/docs/commands). <br> (You will see more options there as we currently support only a subset.)
```

A minimum configuration must include the `endpoint` and `token`.
However, just that would not be secure, as it would not establish an
encrypted TLS connection to Vault. For production you should configure
your Vault securely, following recommendations in the upstream Vault
documentation.

The `kubernetes` backend supports the following configuration keys:

|||
|---|---|
| `ca-cert`| A PEM-encoded CA certificate. This comes from base64 decoding the `certificate-authority-data` value in the kubeconfig file.|
| `ca-certs`| A list of PEM-encoded CA certificates (if a cert chain is used).|
| `client-cert`| A PEM-encoded client certificate. This comes from base64 decoding the user's `client-certificate-data` value in the kubeconfig file.|
| `client-key`| An unencrypted, PEM-encoded client private key. This comes from base64 decoding the user's `client-key-data` value in the kubeconfig file.|
| `endpoint`||
| `namespace`| The namespace to use store the secrets. The namespace must already exist (it is not created).|
| `service-account`| The service account for the access token refresh.|
| `skip-tls-verify`| Do not verify the TLS certificate. For testing only.|
| `token`| The Kubernetes authentication token (can be generated using `kubectl create token ${service-account} --namespace ${namespace}`.|
| `username`| The Kubernetes authentication username.|
| `password`| The Kubernetes authentication password.|

A minimum configuration must include the `endpoint`, `namespace`, and
`ca-cert`.
In most cases, `token` would also be specified for authentication.
If the token is to expire and needs to be rotated, the token's service
account must be specified so a new token can be created.

The service account used to generate the access token must have been
configured with a cluster role binding to allow the necessary access
privileges.
The following is an example of how this might be done:

```
kubectl create clusterrole juju-secrets --verb='*' --resource=namespaces,clusterroles,clusterrolebindings,secrets,serviceaccounts,serviceaccounts/token
kubectl create clusterrolebinding juju-secrets --clusterrole=juju-secrets --serviceaccount=${namespace}:${serviceaccount}
```

Changing a model's active backend moves the model's secrets: the
controller drains the revisions from the old backend into the new one
and rewrites the payload references.

(the-secret-watchers)=
### Secret watchers

The secret domain's watchable service exposes these watch surfaces,
what a watcher fires on:

- **Consumed secrets changes:** a consumer's tracked secret got a new
  revision; this is what delivers the `secret-changed` event (see
  {ref}`the unit agent <unit-agent>`).
- **Obsolete secrets** and **obsolete user secrets to prune:** the
  revisions no consumer tracks; the owner's `secret-remove` and the
  auto-prune housekeeping.
- **Deleted secrets:** secrets removed entirely.
- **Secret revisions' expiry changes:** the expiry clock; the unit
  agent's expiry worker turns it into the `secret-expired` event.
- **Secrets' rotation changes:** the rotation clock; the unit agent's
  rotation worker turns it into the `secret-rotate` event.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-secrets-execution-rules)=
### Execution rules and errors

- **Rules:**
  - Access is checked on every operation: the owner can manage its
    secret (update, grant, revoke, inspect); anyone else needs a
    granted role, view to get the payload; the owner application's
    units read it implicitly.
  - On an application-owned secret, management and the rotate, expire
    and remove hooks run on the leader unit only.
  - A grant cannot change its scope or subject type; a role change is
    an update of the role alone.
  - Backend write authority is granted only for the secret IDs a unit
    actually reserved.
- **Errors:**
  - **`permission denied`:** Triggered when the caller lacks the role
    the operation requires. Remediation: ask the owner to grant the
    needed role.
  - **`cannot change a secret permission scope or subject type`:**
    Triggered when a grant is updated with a different scope or
    subject type. Remediation: revoke the grant and grant it again
    with the scope or subject wanted.
