---
myst:
  html_meta:
    description: "Juju credentials reference: authentication material for cloud access. Credential declaration, persistence (records, types), execution (checks, watchers), and rules."
---

(credential)=
# Credential

In Juju, a **credential** represents a collection of authentication material (like username & password, or client id & secret key) that is specific to a Juju {ref}`user <user>` and a {ref}`cloud <cloud>` and allows that user to interact with that cloud.

Clouds decide which authentication schemes they accept; users own credentials; models use exactly one cloud/credential pair; and who may use a credential is decided by access grants, not by the credential itself.

## Credential in the declaration layer

How clients add a credential for a cloud and manage its life.

- **Adding, updating, removing:** You add, update, or remove a credential through a Juju client; adding your own credential requires nothing beyond controller {ref}`login access <user-access-controller-login>`.
- **Client and controller credentials:** A **client credential** is one the client is aware of; a **controller credential** is one a controller is aware of. Bootstrapping a controller with a client credential uploads it to the controller, after which both sides know it, and the two sets do not have to coincide.

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(the-credential-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - A credential is not a permission of its own: what the credential can do on the cloud is decided by the cloud (how it was created there); what you can do with it through Juju is decided by Juju: you need to own the credential and hold `add-model` or `admin` access on its cloud.
- **Errors:**
  - **`unknown cloud`:** Triggered when naming a cloud the controller does not know. Remediation: add the cloud first.
  - **`user not found`:** Triggered when the credential's owner does not exist. Remediation: check the owner's name.

## Credential in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Credential chain
:no-legend:
:caption: The credential chain lives in the controller DB: a user owns 0..N cloud credentials (cloud/owner/name is the natural key; 15 auth types); a cloud defines 0..N credentials; a model uses 0..1 credential and belongs to one cloud. The model DB carries only a read-only denormalised copy (credential owner/name as text). Access grants are a separate permission record set (there is no credential object type).
:alt: User record, cloud record, credential record, and model record with FK arrows.
```

The authoritative record lives in the {ref}`controller database <database>`:

- **The credential is one record,** identified by its natural key, the {ref}`cloud <cloud>`, the owning {ref}`user <user>`, and its name, unique by index; the record carries its authentication type.
- **The credential's attributes are their own records:** the key/value pairs the cloud's authentication type requires.
- **The model's copy:** The model database keeps only a read-only, denormalised copy of the credential each model uses; identity decisions stay with the controller record.

The natural key is unique: re-adding a credential with the same key updates it, it does not duplicate.

Writers: the credential service in the controller performs the writes; it inserts the record with its attributes when a credential is added, updating is an upsert of the same natural key, removing deletes it, and invalidating marks it invalid with a reason, the models using it being told at their next check.

(the-credential-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - A credential carries two standing flags rather than a life cycle: **revoked** (the user withdrew it) and **invalid** (the cloud or the model checks found it unusable, with the reason recorded). Adding a credential already marked invalid is rejected.
  - A credential's type is its **authentication type**, the scheme the cloud authenticates with, stored on the record itself. Juju seeds one shared list of them: `access-key`, `instance-role`, `userpass`, `oauth1`, `oauth2`, `jsonfile`, `clientcertificate`, `httpsig`, `interactive`, `empty`, `certificate`, `oauth2withcert`, `service-principal-secret`, `managed-identity`, `service-account`. Which of these a given cloud admits, and which attributes each requires, is that cloud's business: see the relevant {ref}`cloud reference page <list-of-supported-clouds>` for details.
- **Errors:**
  - **`credential not found`:** Triggered when querying a credential that does not exist. Remediation: check the credential's cloud, owner, and name.
  - **`model credential not set`:** Triggered when a model operation needs the model's credential and none is set. Remediation: set the model's credential.
  - **`credential is not valid for one or more models`:** Triggered when a cloud-side check finds the credential unusable for the models using it. Remediation: fix or replace the credential.

## Credential in the execution layer

By the time the setting call returns, the record exists, and nothing has yet proved that the credential works. That proof is all the execution a credential has, because a credential has no machinery of its own: opening a provider connection with the model's credential is what validates it, per model, at the model's next check.

(the-credential-watchers)=
### Credential watchers

One watch surface: **a single credential's changes**, the provisioning machinery of a model that uses the credential watches it and reconciles when the credential is updated or invalidated.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.