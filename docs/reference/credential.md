---
myst:
  html_meta:
    description: "Juju credentials reference: authentication material for cloud access. Credential declaration, persistence (records, types), execution (checks, watchers), and rules."
---

(credential)=
# Credential

In Juju, a **credential** represents a collection of authentication material (like username & password, or client id & secret key) that is specific to a Juju {ref}`user <user>` and a {ref}`cloud <cloud>` and allows that user to interact with that cloud.

Clouds decide which authentication schemes they accept; users own credentials; models use exactly one cloud/credential pair; and who may use a credential is decided by access grants, not by the credential itself.

(the-credential-declaration-rules)=
## Credential in the declaration layer

How clients add a credential for a cloud and manage its life.

- **Adding, updating, removing:** You add, update, or remove a credential through a Juju client; adding your own credential requires nothing beyond controller {ref}`login access <user-access-controller-login>`.
  - *Rule:* A credential is not a permission of its own. The cloud decides what the credential can do on the cloud (how it was created there); Juju decides what you can do with it through Juju: you need to own the credential and hold `add-model` or `admin` access on its cloud.
  - *Rule:* Updating a credential checks it against every model that uses it, unless the update is forced.
  - *Related errors:*
    - **`unknown cloud`**: *Trigger:* Naming a cloud the controller does not know. *Remediation:* Add the cloud first.
    - **`user not found`**: *Trigger:* The credential's owner does not exist. *Remediation:* Check the owner's name.
    - **`credential is not valid for one or more models`**: *Trigger:* The check on update finds the credential unusable for a model that uses it. *Remediation:* Fix or replace the credential, or force the update.
- **Client and controller credentials:** A **client credential** is one the client is aware of; a **controller credential** is one a controller is aware of. Bootstrapping a controller with a client credential uploads it to the controller, after which both sides know it, and the two sets do not have to coincide.

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(the-credential-persistence-rules)=
## Credential in the persistence layer

The authoritative record lives in the {ref}`controller database <database>`:

- **Credential record:** One record, identified by its natural key: the {ref}`cloud <cloud>`, the owning {ref}`user <user>` and its name, unique by index. The record carries its authentication type. Re-adding a credential with the same key updates it and does not duplicate it.
  - *Related error:*
    - **`credential not found`**: *Trigger:* A lookup by cloud, owner and name that matches no credential. *Remediation:* Check the credential's cloud, owner and name.
- **Attribute records:** The key/value pairs that the cloud's authentication type requires, each its own record.
- **Standing flags:** Two flags take the place of a life cycle. A credential is **revoked** when the user withdrew it, and **invalid** when the cloud or the model checks found it unusable; the record keeps the reason.
- **Authentication type:** The scheme the cloud authenticates with, stored on the record itself. Juju seeds one shared list: `access-key`, `instance-role`, `userpass`, `oauth1`, `oauth2`, `jsonfile`, `clientcertificate`, `httpsig`, `interactive`, `empty`, `certificate`, `oauth2withcert`, `service-principal-secret`, `managed-identity`, `service-account`. Which of these a given cloud admits, and which attributes each requires, is that cloud's business: see the relevant {ref}`cloud reference page <list-of-supported-clouds>` for details.
- **The model's copy:** The model database keeps only a read-only, denormalised copy of the credential each model uses (owner and name as text); identity decisions stay with the controller record.
  - *Related error:*
    - **`model credential not set`**: *Trigger:* A model operation needs the model's credential and none is set. *Remediation:* Set the model's credential.

**Writers:** The credential service in the controller performs the writes. Adding inserts the record with its attributes; updating is an upsert of the same natural key; removing deletes it; invalidating marks it invalid with a reason, and the models using it learn of it at their next check.

## Credential in the execution layer

By the time the setting call returns, the record exists, and nothing has yet proved that the credential works. That proof is all the execution a credential has, because a credential has no machinery of its own: opening a provider connection with the model's credential is what validates it, per model, at the model's next check.

(the-credential-watchers)=
### Credential watchers

One watch surface: **a single credential's changes**. The provisioning machinery of a model that uses the credential watches it and reconciles when the credential is updated or invalidated.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
