---
myst:
  html_meta:
    description: "Juju credentials reference: authentication material for cloud access, credential types, storage, and model associations. The credential record, types, states, operations, and rules."
---

(credential)=
# Credential
```{audience} user
```

```{ibnote}
See also: {ref}`manage-credentials`
```

In Juju, a **credential** represents a collection of authentication material (like username & password, or client id & secret key) that is specific to a Juju {ref}`user <user>` and a {ref}`cloud <cloud>` and allows that user to interact with that cloud.

```{important}
In Juju a 'credential' always refers to authentication material used to access a *cloud*.
```

Clouds can have one or more sets of credentials associated with them.

When you create a  {ref}`model <model>` in Juju it must always be associated with a cloud/credential pair -- the model needs that to create resources on the underlying cloud.

(the-credentials-records)=
## The credential's records

(the-credential-record)=
### The credential's identity

In the **controller** database, a credential is a record identified by
its natural key -- the {ref}`cloud <cloud>`, the owning
{ref}`user <user>`, and its name -- plus its authentication type and
its attributes (the key/value pairs the cloud's auth type requires).
The record carries the credential's standing: whether it has been
revoked, and whether it has been found **invalid**, with the reason.
The model database keeps only a read-only, denormalised copy of the
credential each model uses.

(the-credential-in-the-data-model)=
### The credential in the data model

```{ggarch}
:file: ../juju.ggarch
:view: Credential chain
:no-legend:
:caption: Entity relationship diagram: The credential chain lives in the controller DB: a user owns 0..N cloud credentials (cloud/owner/name is the natural key; 15 auth types); a cloud defines 0..N credentials; a model uses 0..1 credential and belongs to one cloud. The model DB carries only a read-only denormalised copy (credential owner/name as text). Access grants are a separate permission table (there is no credential object type; credential access is ownership plus cloud-level add-model/admin).
:alt: User record, cloud record, credential record, and model record with FK arrows.
```

(the-credential-states)=
### Credential states

A credential carries two standing flags rather than a life cycle:
**revoked** (the user withdrew it) and **invalid** (the cloud or the
model checks found it unusable, with the reason recorded). Adding a
credential already marked invalid is rejected; the real validity
check is per model -- opening a provider connection with the
credential is what proves it.

(types-of-credential)=
### Types of credential

A credential's type is its **authentication type** -- the scheme the
cloud authenticates with. Juju seeds one shared list of them:
`access-key`, `instance-role`, `userpass`, `oauth1`, `oauth2`,
`jsonfile`, `clientcertificate`, `httpsig`, `interactive`, `empty`,
`certificate`, `oauth2withcert`, `service-principal-secret`,
`managed-identity`, `service-account`. Which of these a given cloud
admits -- and which attributes each requires -- is that cloud's
business: see the relevant {ref}`cloud reference page
<list-of-supported-clouds>` for details.

(client-vs-controller-credential)=
#### Client vs. controller credential

Juju credentials can be created for either the Juju client or the Juju controller or both -- where a **client credential** (previously known as a 'local credential') denotes a credential that the client is aware of and a **controller credential** (previously known as a 'remote credential') denotes a credential that a controller is aware of. When you bootstrap a controller and use a client credential, this credential gets automatically uploaded to the controller, so it becomes a controller credential also.

```{important}
The set of client credentials and controller credentials can end up being the same. However, they don't have to.
```

(the-credentials-machinery)=
## The credential's machinery

A credential has no machinery of its own: the controller stores the
records and validates them against their cloud; the one watch surface
(credential changes) reports the stored set.

(the-credential-operations)=
### Credential operations

Adding a credential (`juju add-credential`) inserts the record with
its attributes; updating is an upsert of the same natural key;
removing deletes it; invalidating marks it invalid with a reason and
the models using it are told at their next check. The
check-credentials operation validates, per model, that the model's
credential actually opens the cloud.

(the-credential-watchers)=
### Credential watchers
```{audience} juju-dev
```

One watch surface: **a single credential's changes** -- the
provisioning machinery of a model that uses the credential watches it
and reconciles when the credential is updated or invalidated.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-credential-rules-and-errors)=
## Credential rules and errors

- the natural key -- cloud, owner, name -- is unique: re-adding
  updates, it does not duplicate;
- the authentication type must be one the cloud admits, and the
  attributes must satisfy it;
- a credential recorded as invalid cannot be added;
- a credential is not a permission of its own: it only authenticates
  to the cloud. What it can do on the cloud is decided by the cloud
  (how it was created there); what you can do with it through Juju is
  decided by Juju -- you need to own the credential and hold
  `add-model` or `admin` access on its cloud;

The errors that encode them: `credential not found`,
`credential model validation failed`, `model credential not set`,
`unknown cloud`, `user not found`.

(related-entities-credential)=
## Entities related to the credential

- A credential authenticates against a **cloud**, and the cloud
  defines the admitted auth types (see {ref}`cloud <cloud>`).
- **Users** own credentials -- the natural key's owner half (see
  {ref}`user <user>`).
- **Models** use exactly one cloud/credential pair, recorded on the
  model (see {ref}`model <model>`).
