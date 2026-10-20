---
myst:
  html_meta:
    description: "Juju secret reference: manage sensitive data like credentials, API keys, and certificates. Learn charm secrets, user secrets, and secret backends."
---

(secret)=
# Secret

```{ibnote}
See also: {ref}`manage-secrets`
```

In Juju, a **secret** is a sensitive bit of information (e.g., account credential, password, certificate, SSH key, API key, encryption key, etc.) that a {ref}`charm <charm>` needs to know.

## Secret taxonomy

(charm-secret)=
### Charm secret

A **charm secret** is a secret created by a charm. A charm secret is shared with another charm (the secret 'observer') over relation data. The secret is tied to the lifecycle of the relation.

(unit-secret)=
#### Unit secret

A **unit secret** is a {ref}`charm secret <charm-secret>` created by a unit and owned by the unit.

(application-secret)=
#### Application secret

An **application secret** is a {ref}`charm secret <charm-secret>` created by the leader unit and (because the leader unit does not have a fixed identity) owned by the application (i.e., when the leader unit changes, the secret is owned by the new leader).

(user-secret)=
### User secret

A **user secret** is a secret created by a {ref}`user <user>` with a {ref}`model admin access level <user-access-model-admin>` and (because this does not have a fixed identity) owned by the model. A user secret is shared with a charm (the secret 'observer') via a configuration option. The charm must support the configuration option.

## Secret identification

Secrets are identified by an automatically assigned URI (see more: {ref}`secret-uri`).

The secret URI can always be used in any content to identify a secret. Additionally, users give secrets they create names (see more: {ref}`secret-name`), and charms may choose to assign labels to secrets (see more: {ref}`secret-label`).

In short, the same secret may end up being associated with multiple identifiers. For example:

- (name vs. URI vs. label:) I as a user might create a secret with a name that makes sense to me, for example, `my-api-key`. Juju assigns it a URI, for example, `9m4e2mr0ui3e8a215n4g`. When I then configure a charm to use it, the charm might give it a label, for example, `vault-api-token`.
- (label vs. URI vs. label:) A leader unit creates an application secret and assigns to it a label in its capacity as the secret owner, for example, `db-password`. Juju assigns it a URI, e.g., `6k7n4ps1vj2d9b318x5w`. Any unit granted permission to the secret (peer units get implicit permission) might assign another label in their capacity as secret consumers, for example, `shared-db-creds`.

(secret-uri)=
### Secret URI

In both {ref}`user secrets <user-secret>` and {ref}`charm secrets <charm-secret>`, a secret URI is automatically assigned by Juju when the secret is added, either by a user through `juju add-secret` (or its equivalent in other Juju clients) or by a charm via `secret-add`. The URI is returned to the caller so they can then use it in subsequent actions (for example, a charm might grant permission to that secret using the URI and then put the URI in relation data).

(secret-name)=
### Secret name

In {ref}`user secrets <user-secret>`, a secret name is the string identifier assigned to a secret by the user when adding the secret to Juju, for their own reference.

Secret names must start with a lowercase letter, followed by a sequence of letters, numbers, and dashes, and must not end with a dash; in short, they must comply with the following regex: `^([a-z](?:-?[a-z0-9]){2,})$`.

(secret-label)=
### Secret label

In {ref}`user secrets <user-secret>` or {ref}` charm secrets <charm-secret>`, a secret label is a string identifier that may be assigned to a secret by the secret owning and, respectively, the secret consuming charm for their own internal reference.

Unlike secret names, labels have no format constraints and can be any string. This flexibility allows charms to use their own naming conventions for internal reference.

## Secret size

The maximum size for a base64-encoded secret value is `1MB` (1,000,000 bytes) per key. This limit ensures compatibility across all secret backends, including Vault and Kubernetes.

## Secret backend

A secret's content lives in a {ref}`secret backend <secret-backend>`, which each model chooses. See the {ref}`secret backend <secret-backend>` page for the backends, their configuration options, and the operations on them.

## Permissions around secrets

An entity -- unit/app or user -- that has created / owns the secret can **manage** it (call `secret-set, secret-grant, secret-revoke, secret-info-get`, etc.).

An entity that does not own the secret can only **view** it (call `secret-get`), and only if it has been granted access to it -- except for peer units or a model admin user, who get view access automatically.

## Secret lifecycle

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

