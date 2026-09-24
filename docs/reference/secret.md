---
myst:
  html_meta:
    description: "Juju secret reference: the secret record, secret kinds (charm and user secrets), the secret in the data model, secret states, operations (backends, rotation, ACLs), watchers, and rules."
---

(secret)=
# Secret

```{ibnote}
See also: {ref}`manage-secrets`, {ref}`manage-secret-backends`
```

In Juju, a **secret** is a piece of sensitive information -- an API key, a password, a certificate -- that Juju stores on behalf of its owner and shares only with the consumers it is granted to.

(the-secret-record)=
## The secret record

A secret is identified by its **secret ID** -- the identifier part of
its URI -- and it carries its metadata (a description, its rotation
policy, whether its obsolete revisions are auto-pruned, and the latest
revision number), an **owner** record (the application, unit, or model
that created it, with the owner's label for it), and its **revisions**
-- one record per published payload, each with its content either
stored inline or referenced in a {ref}`secret backend <secret-backend>`,
plus its obsolete and expiry bookkeeping (see
{ref}`Secret states <the-secret-states>`). Grants are their own
records (see {ref}`the data model <the-secret-in-the-data-model>`).

The same secret may end up being associated with multiple identifiers:

- (name vs. URI vs. label:) I as a user might create a secret with a name that makes sense to me, for example, `my-api-key`. Juju assigns it a URI, for example, `9m4e2mr0ui3e8a215n4g`. When I then configure a charm to use it, the charm might give it a label, for example, `vault-api-token`.
- (label vs. URI vs. label:) A leader unit creates an application secret and assigns to it a label in its capacity as the secret owner, for example, `db-password`. Juju assigns it a URI, e.g., `6k7n4ps1vj2d9b318x5w`. Any unit granted permission to the secret (peer units get implicit permission) might assign another label in their capacity as secret consumers, for example, `shared-db-creds`.

(secret-uri)=
### Secret URI

In both {ref}`user secrets <user-secret>` and {ref}`charm secrets <charm-secret>`, a secret URI is automatically assigned by Juju when the secret is added, either by a user through `juju add-secret` (or its equivalent in other Juju clients) or by a charm via `secret-add`. The URI is returned to the caller so they can then use it in subsequent actions (for example, a charm might grant permission to that secret using the URI and then put the URI in relation data).

The URI's ID part is a 20-character identifier; a secret that
originated in another model carries that model's UUID as the URI's
host part (`secret://<source-uuid>/<id>`); a local secret's URI is
just `secret:<id>`.

(secret-name)=
### Secret name

In {ref}`user secrets <user-secret>`, a secret name is the string identifier assigned to a secret by the user when adding the secret to Juju, for their own reference.

Secret names must start with a lowercase letter, followed by a sequence of letters, numbers, and dashes, and must not end with a dash; in short, they must comply with the following regex: `^([a-z](?:-?[a-z0-9]){2,})$`.

(secret-label)=
### Secret label

In {ref}`user secrets <user-secret>` or {ref}` charm secrets <charm-secret>`, a secret label is a string identifier that may be assigned to a secret by the secret owning and, respectively, the secret consuming charm for their own internal reference.

Unlike secret names, labels have no format constraints and can be any string. This flexibility allows charms to use their own naming conventions for internal reference.

(types-of-secret)=
## Types of secret

A secret is typed by **who owns it** -- the owner record is one of an
application, a unit, or the model, and that is the whole taxonomy.

(charm-secret)=
### Charm secret

```{versionadded} 3.0.0
```

A **charm secret** is a secret created by a charm. A charm secret is shared with another charm (the secret 'observer') over relation data. The secret is tied to the lifecycle of the relation.

(unit-secret)=
#### Unit secret

A **unit secret** is a {ref}`charm secret <charm-secret>` created by a unit and owned by the unit.

(application-secret)=
#### Application secret

An **application secret** is a {ref}`charm secret <charm-secret>` created by the leader unit and (because the leader unit does not have a fixed identity) owned by the application (i.e., when the leader unit changes, the secret is owned by the new leader).

(user-secret)=
### User secret

```{versionadded} 3.3.0
```

A **user secret** is a secret created by a {ref}`user <user>` with a {ref}`model admin access level <user-access-model-admin>` and (because this does not have a fixed identity) owned by the model. A user secret is shared with a charm (the secret 'observer') via a configuration option. The charm must support the configuration option.

(the-secret-in-the-data-model)=
## The secret in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Secret attributes
:alt: The secret's stored tables as an entity-relationship slice: the metadata record (keyed by the secret id) at the centre; the revision chain and its content west; the owner and the consumers east; the permission grants south. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The secret's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The metadata record is keyed by the secret ID; the owner (application | unit | model) and the consumers carry the labels; revisions chain off the secret, each storing its payload either inline or as a backend reference; permission grants hang off the secret itself.
```

The secret's records are split by role: the `secret_metadata` record
(keyed by the secret ID) carries the description, the rotation policy,
the auto-prune flag and the latest revision number; the owner records
-- one table each for an application, unit, or model owner -- carry
the owner and its label (unique per owner); the consumer records carry
the consumers and their labels; the `secret_revision` records carry
the published payloads, each with its content in the `secret_content`
records (inline, for the internal backend) or in a
`secret_value_ref` (a pointer into the
{ref}`secret backend <secret-backend>`'s store), and their obsolete,
pending-delete and expiry bookkeeping; and the `secret_permission`
records carry the grants (see
{ref}`rules <the-secret-rules-and-errors>`). The backends themselves
-- the types, their configurations, and the per-model active backend
-- live in the controller database, not the model database.

(the-secret-states)=
## Secret states

```{ggarch}
:file: ../juju.ggarch
:view: Secret lifecycle
:no-legend:
:caption: State machine diagram: The life of a secret, grounded in domain/secret: reserved (URI minted) -> active (latest revision, content in a backend) -> granted (view | manage roles) -> superseded; a rotate policy fires secret-rotate (leader), expiry fires secret-expired; a revision no consumer tracks becomes obsolete (pending delete) and the owner charm retires it via secret-remove (or user secrets auto-prune). Consumers see secret-changed.
:alt: State machine: reserved to active on create, active self-loops for grant/revoke and new-revision publication, active to rotate-due on the rotate policy and back via secret-rotate, active to expiry-due and on to removed via secret-expired then secret-remove, active to obsolete when superseded, obsolete to removed on prune.
```

A secret carries no life cycle of the shared alive / dying / dead
kind: its state machine is the revision and access story -- reserved,
active, granted, superseded, obsolete, removed (the view above).
The state is driven by the owner's and the consumers' operations
(see {ref}`Secret operations <the-secret-operations>`) and observed
through the secret events (see
{ref}`the secret hooks <hook-secret-changed>`).

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

(the-secret-operations)=
## Secret operations

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
## Secret watchers

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

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

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

(related-entities-secret)=
## Related entities

- **Applications and units** own charm secrets and consume them as
  observers; the leader owns the application's secrets
  (see {ref}`application <application>`, {ref}`unit <unit>`).
- **Relations** scope charm-secret access: removing the relation
  revokes the access it carried (see {ref}`relation <relation>`).
- **Users** own model secrets and manage them (see
  {ref}`user <user>`).
- **Backends** store the payloads: the controller database (internal),
  the model's Kubernetes namespace, or Vault (see
  {ref}`secret backend <secret-backend>`).
- **The unit agent** delivers the secret events: `secret-changed`,
  `secret-rotate` (leader), `secret-expired` (see
  {ref}`unit agent <unit-agent>`,
  {ref}`hook secret-changed <hook-secret-changed>`).
- **Removal** cleans up: an application's or unit's removal deletes
  the secrets it owns (see
  {ref}`application removal <the-application-removal>`).
