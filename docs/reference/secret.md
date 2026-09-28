---
myst:
  html_meta:
    description: "Juju secret reference: the secret record, secret kinds (charm and user secrets), the secret in the data model, secret states, operations (backends, rotation, ACLs), watchers, and rules."
---

(secret)=
# Secret

In Juju, a **secret** is a piece of sensitive information -- an API key, a password, a certificate -- that Juju stores on behalf of its owner and shares only with the consumers it is granted to. Its owner
records tie it to the {ref}`application <application>` or {ref}`unit <unit>` that
created it -- and an application's or unit's removal deletes the secrets it owns
-- a charm secret's {ref}`relation <relation>` scope revokes its access when the
relation goes, and its payloads live in a {ref}`secret backend <secret-backend>`.

(the-secrets-declaration)=
## Secrets in the declaration layer

A charm declares its secrets through its hook context -- `secret-add`,
`secret-set`, `secret-grant`, `secret-revoke`, `secret-info-get`; a user
declares user secrets through a Juju client -- `juju add-secret`,
`juju update-secret`, `juju grant-secret`, `juju revoke-secret`,
`juju remove-secret`; user-secret mutations require {ref}`model write access
<user-access-model-write>`.

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`, {ref}`Terraform Provider for Juju | Manage secrets <tfjuju:manage-secrets>`
```

(the-secrets-persistence)=
(the-secrets-records)=
(the-secret-record)=
(the-secret-in-the-data-model)=
(the-secret-states)=
(secret-uri)=
(secret-name)=
(secret-label)=
## Secrets in the persistence layer

A secret is a Juju-managed payload with controlled access: the
metadata record and everything hanging off it live in the model
database (the DDL: `0012-secret.sql`); the backends themselves -- the
types, their configurations, and the per-model active backend -- live
in the controller database, not the model database. A secret is
identified by its **secret ID** -- the identifier part of its URI --
and it carries its metadata (a description, its rotation policy,
whether its obsolete revisions are auto-pruned, and the latest
revision number), an **owner** record (the application, unit, or model
that created it, with the owner's label for it), and its **revisions**
-- one record per published payload, each with its content either
stored inline or referenced in a {ref}`secret backend
<secret-backend>`, plus its obsolete and expiry bookkeeping. Grants
are their own records.

```{ggarch}
:file: ../juju.ggarch
:view: Secret attributes
:alt: The secret's stored tables as an entity-relationship slice: the metadata record (keyed by the secret id) at the centre; the revision chain and its content west; the owner and the consumers east; the permission grants south. Each line is a stored pointer; 1/m at each end; nothing dashed -- every pointer here is mandatory (the owner and consumer labels are nullable fields, not pointers).
:caption: Entity relationship diagram: The secret's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the row may be absent). The metadata record is keyed by the secret ID; the owner (application | unit | model) and the consumers carry the labels; revisions chain off the secret, each storing its payload either inline or as a backend reference; permission grants hang off the secret itself.
```

The identity pair: the primary key is the secret ID itself
(`secret_metadata.secret_id`) -- there is no separate join handle: the
owner records, the revisions, the consumers and the permission grants
all point at the secret ID. The natural keys are the client-facing
handles on that one ID: the **secret URI** -- assigned by Juju when
the secret is added (by a user through `juju add-secret` or a charm
via `secret-add`; returned to the caller for subsequent actions, for
example granting permission or putting the URI in relation data). The
URI's ID part is a 20-character identifier; a secret that originated
in another model carries that model's UUID as the URI's host part
(`secret://<source-uuid>/<id>`); a local secret's URI is just
`secret:<id>`. A user secret may also carry a user-chosen **name**
(`my-api-key`, matching `^([a-z](?:-?[a-z0-9]){2,})$`) -- the string
identifier for the user's own reference; and an owner or consumer may
assign a **label** (`vault-api-token`) -- any string, no format
constraints, unique per owner or consumer, for the charm's internal
reference. One secret may end up with all of these at once: the user
names it at create time, Juju assigns the URI, the consuming charm
labels it.

Every foreign key is an assertion the record holds: the owner rows
hold the secret pointer (one table each for an application, unit, or
model owner -- which table is the secret's type, read off the ERD
below) and their owner pointers name the owning application, unit or
model; the revision rows hold the secret pointer (UNIQUE on secret ID
+ revision number); the content rows hold the revision pointer (the
payload inline, for the internal backend) and a `secret_value_ref`
row holds the backend reference when the payload lives in a
{ref}`secret backend <secret-backend>`'s store; the consumer rows
hold the secret pointer and record the tracked revision per unit (or
per remote unit, for cross-model secrets); the permission rows hold
the secret pointer and carry the grant: the role (none | view |
manage) over a subject (unit | application | model) within a scope
(unit | application | model | relation) (see
{ref}`rules <the-secret-rules-and-errors>`).

The same secret may end up being associated with multiple identifiers:

- (name vs. URI vs. label:) I as a user might create a secret with a name that makes sense to me, for example, `my-api-key`. Juju assigns it a URI, for example, `9m4e2mr0ui3e8a215n4g`. When I then configure a charm to use it, the charm might give it a label, for example, `vault-api-token`.
- (label vs. URI vs. label:) A leader unit creates an application secret and assigns to it a label in its capacity as the secret owner, for example, `db-password`. Juju assigns it a URI, e.g., `6k7n4ps1vj2d9b318x5w`. Any unit granted permission to the secret (peer units get implicit permission) might assign another label in their capacity as secret consumers, for example, `shared-db-creds`.

A secret is typed by **who owns it** -- the owner record is one of an
application, a unit, or the model, and that is the whole taxonomy. The
type is a stored discriminator: it is which owner table the secret
hangs off.

```{ggarch}
:file: ../juju.ggarch
:view: Secret lifecycle
:no-legend:
:caption: State machine diagram: The life of a secret, grounded in domain/secret: reserved (URI minted) -> active (latest revision, content in a backend) -> granted (view | manage roles) -> superseded; a rotate policy fires secret-rotate (leader), expiry fires secret-expired; a revision no consumer tracks becomes obsolete (pending delete) and the owner charm retires it via secret-remove (or user secrets auto-prune). Consumers see secret-changed.
:alt: State machine: reserved to active on create, active self-loops for grant/revoke and new-revision publication, active to rotate-due on the rotate policy and back via secret-rotate, active to expiry-due and on to removed via secret-expired then secret-remove, active to obsolete when superseded, obsolete to removed on prune.
```

One state machine, and it is not the shared life:

- **the revision and access story** -- reserved (URI minted), active
  (the latest revision published, its content in a backend), granted
  (view | manage roles), superseded, obsolete, removed (the view
  above). A secret carries no life cycle of the shared alive / dying /
  dead kind. The operations in the execution layer drive it, the
  records here store it; it is observed through the secret events
  (see {ref}`the secret hooks <hook-secret-changed>`).

A secret's state machine has writers, but not column writers the way
a status table has: the owner publishes revisions, the controller
writes the grant and tracking rows, the rotate policy and the expiry
fire the owner's hooks. The projection above is those writers' story.

### Charm-secret lifecycle

Charms can use relations to share secrets, such as API keys, a database's address, credentials and so on. Like a relation has a "provider" and a "requirer", so a secret has an "owner" and an "observer" -- though these need not coincide with the applications' roles in the relation.

Every secret has a **scope**, and that is the relation its lifecycle is tied to. If the relation is removed, the secret access will be revoked.

When a unit adds a secret, it becomes that secret's **owner**, and it will obtain from Juju a **secret ID**, which it can then pass to some remote application via relation data. Any remote unit with access to that ID can get the secret (that is, access its contents). Before that is possible, however, the owner needs to **grant** the secret to the whole application or a specific unit.

Once the remote unit (the secret **observer**) gets its contents for the first time, it starts to track that secret in Juju -- more specifically, its *latest revision*, as we will see later. When the owner adds the secret, and when the observer gets it, they both have a chance to assign to the secret a **label**, a locally-unique string that will be associated with that secret and can be used by the charm to refer to the secret "by name".

The owner can choose at any time to publish a new **revision** of the secret, that is, change its payload (for example, replace an old key with a new one). When that happens, the observer will be notified by means of a `secret-changed` event. The observer can then **refresh** the secret, which means inform Juju that it wishes to start tracking the latest revision. From that moment on, every time the unit gets the secret, it will receive the newly-tracked revision's contents.

However, a unit does not have to immediately update whenever a new revision becomes available. It can **peek** the secret's contents, which means to inspect the latest revision of the secret without updating to it, or choose to do nothing.

```{important}

When a unit gets a secret for the first time it will automatically be set to track the latest revision. A unit cannot choose to track an outdated revision, but it can in principle refuse to update to a newer one.

```

When a charm secret is added, the owner can configure it to have a **rotation** policy (hourly, daily, monthly, and so on). In that case, the owner will be periodically notified, by means of a `secret-rotate` event, that it is time to rotate the secret -- that is, create a new revision for it.

Alternatively, a charm secret can be configured to have an **expiration** date, that is, a specific point in time at which the charm will be notified by Juju that it is time to retire the secret by means of a `secret-expired` event.

Juju maintains a list of which observers are tracking each revision of each secret. The idea is that if an observer receives a `secret-changed` event, it will update the secret and start tracking the latest revision. Once Juju notices that there are no observers left for a given revision, it will notify the secret owner that that secret revision can be safely **removed** -- which corresponds to the `secret-remove` event.

```{important}

Charms that create secrets should _always_ handle the `secret-remove` event. That is because secret revisions, even if obsolete, remain until removed by the charm; if a charm does not remove them, they accumulate indefinitely.

```

### User-secret lifecycle

A user secret's lifecycle consists of:

1. **Create**: A model admin creates the secret: `juju add-secret <name> <key>=<value>`.
2. **Grant**: The admin grants access to an application: `juju grant-secret <name> <app-name>` — this does **not** fire a hook on the observing charm.
3. **Configure**: The admin sets the application's configuration option to the secret URI: `juju config <app-name> <option>=<secret-uri>` — this triggers a `config-changed` hook on the observing charm.
4. **Update**: The admin updates the secret content: `juju update-secret <name> <key>=<new-value>` — this triggers `secret-changed` on all observing units.

The **only hook** a user-secret observer receives is `secret-changed`. There is no `secret-rotate`, `secret-expired`, or `secret-remove` lifecycle for user secrets.

> See also: {ref}`hook-secret-changed`, {ref}`manage-secrets`

(types-of-secret)=
### Types of secret

(charm-secret)=
#### Charm secret

```{versionadded} 3.0.0
```

A **charm secret** is a secret created by a charm. A charm secret is shared with another charm (the secret 'observer') over relation data. The secret is tied to the lifecycle of the relation.

(unit-secret)=
##### Unit secret

A **unit secret** is a {ref}`charm secret <charm-secret>` created by a unit and owned by the unit.

(application-secret)=
##### Application secret

An **application secret** is a {ref}`charm secret <charm-secret>` created by the leader unit and (because the leader unit does not have a fixed identity) owned by the application (i.e., when the leader unit changes, the secret is owned by the new leader).

(user-secret)=
#### User secret

```{versionadded} 3.3.0
```

A **user secret** is a secret created by a {ref}`user <user>` with a {ref}`model admin access level <user-access-model-admin>` and (because this does not have a fixed identity) owned by the model. A user secret is shared with a charm (the secret 'observer') via a configuration option. The charm must support the configuration option.

(the-secrets-machinery)=
(the-secret-operations)=
## Secrets in the execution layer

A secret has machinery of its own: in the controller and its backends the
secret is stored, granted and pruned, and on the owner's unit agent the
rotation and expiry workers turn the policy clocks into the
`secret-rotate` and `secret-expired` events.

Operations on secrets split by concern: creating and updating (the
owner's writes), granting and revoking (the access changes), rotating
and expiring (the policy-driven revisions), removing (the teardown),
and the backend changes.

### Creating and updating secrets

A secret is created by its owner -- a charm (`secret-add`) or a user
(`juju add-secret`): Juju mints the secret ID, writes the metadata and
owner records, and stores the first revision's payload in the active
{ref}`backend <secret-backend>`. Publishing a new revision (a charm's
`secret-set`, a user's `juju update-secret`) adds a revision record
and notifies the consumers (the `secret-changed` event); the consumers
then {ref}`track or peek <hook-secret-changed>` it.

### Granting and revoking access

Access is a grant record on the secret: the owner grants a subject (an
application, a unit, a user, or a model) a role -- `view` or `manage`
-- optionally scoped to a relation, and can revoke it. Peer units of
the owner's application get view access implicitly; a relation's
removal revokes the access it carried.

### Rotating secrets

A rotation policy (hourly, daily, weekly, monthly, quarterly, yearly)
puts the secret on a rotation clock: when it fires, the owner unit's
agent runs the charm's `secret-rotate` hook (leader-gated), the charm
publishes a new revision, and the next rotation time moves on. A
policy and its next rotation time always come as a pair.

### Expiring and removing secrets

An expiry date on a revision fires the `secret-expired` event when it
passes, telling the owner to retire the secret. Revisions no consumer
tracks become obsolete: the charm removes them (`secret-remove`) or,
for secrets with auto-prune, Juju deletes them itself. Removing a
secret deletes its records and its backend payloads.

(secret-backend)=
### Secret backends

```{ibnote}
See also: {ref}`manage-secret-backends`
```

A **secret backend** is a service that is used to store sensitive content which Juju manages as {ref}`secrets <secret>`.

Secret backends are per model.

A secret backend is identified by a name and a type, and admits various configuration options, some of them generic and some backend-type-specific.

#### Name

The name of a secret backend can be:

- `auto` (i.e., `internal` for machine models and `local` for Kubernetes models)
- `internal`
- `<some custom name>`

The name is set via the `secret-backend` model configuration key.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`
```

#### Type

The type of a secret backend can be `controller`, `kubernetes`, and `vault`.

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

It is available as an opt-in to both machine  and Kubernetes models.

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

A minimum configuration must include the `endpoint` and `token`. However, just that would not be insecure, as it wouldn't establish an encrypted TLS connection to Vault. For production you should configure your Vault securely, following recommendations in the upstream Vault documentation.

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
| `token`| The Kuberneres authentication token (can be generated using `kubectl create token ${service-account} --namespace ${namespace}`.|
| `username`| The Kuberneres authentication username.|
| `password`| The Kuberneres authentication password.|

A minimum configuration must include the `endpoint`, `namespace`, and `ca-cert`.
In most cases, `token` would also be specified for authentication.
If the token is to expire and needs to be rotated, the token's service account must be specified so a new token can be created.

The service account used to generate the access token must have been configured with a cluster role binding to allow the necessary access privileges.
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

The secret domain's watchable service exposes these watch surfaces --
what a watcher fires on, not who subscribes beyond the named
consumers:

- **Consumed secrets changes** -- a consumer's tracked secret got a
  new revision (this is what delivers the `secret-changed` event; see
  {ref}`the unit agent <unit-agent>`).
- **Obsolete secrets** and **obsolete user secrets to prune** -- the
  revisions no consumer tracks; the owner's `secret-remove` and the
  auto-prune housekeeping.
- **Deleted secrets** -- secrets removed entirely.
- **Secret revisions' expiry changes** -- the expiry clock; the unit
  agent's expiry worker turns it into the `secret-expired` event.
- **Secrets' rotation changes** -- the rotation clock; the unit
  agent's rotation worker (leader-gated) turns it into the
  `secret-rotate` event.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-secret-rules-and-errors)=
## Secret rules and errors

The rules a **secret identifier** must satisfy:

- the secret name (user secrets) matches the name regex -- lowercase
  letter first, letters, numbers and dashes, no trailing dash;
- labels are unique **per owner scope**: an owner's label and each
  consumer's label are unique among that owner's / consumer's secrets;
- a secret's URI ID is minted by Juju; a cross-model secret's URI
  carries the source model's UUID.

The rules a **secret payload and policy** must satisfy:

- the maximum size for a base64-encoded secret value is `1MB`
  (1,000,000 bytes) per key, for compatibility across all backends,
  including Vault and Kubernetes;
- a rotation policy and a next rotation time always come together --
  one without the other is invalid;
- access is a grant record: the owner can **manage** its secret
  (`secret-set`, `secret-grant`, `secret-revoke`, `secret-info-get`);
  everyone else needs a granted role -- `view` to `secret-get` --
  except peer units and a model admin, who get view implicitly; a
  role change that would leave the permission state inconsistent is
  rejected (`invalid secret permission change`).

The errors that encode them:

- *Existence*: `secret not found`, `secret revision not found`,
  `secret consumer not found`, `secret access scope not found`.
- *Validation*: `secret label already exists`,
  `invalid secret permission change`, `permission denied`,
  `secret is not local`, `auto-prune not supported`,
  `missing secret backend id`.
