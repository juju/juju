---
myst:
  html_meta:
    description: "SSH key reference: the user public keys Juju projects onto machines and the machines' own host keys. The records, the acts on keys, and the key updater and host key reporting machinery."
---

(ssh-key)=
# SSH key

```{ibnote}
See also: {ref}`manage-ssh-keys`
```

An **SSH key** gives its holder shell access to a machine Juju
provisioned. The records come in two families: the {ref}`user <user>`'s
public keys, which Juju projects onto the model's {ref}`machines
<machine>`, and the machines' own SSH **host keys**, the keys the
machines present to whoever connects to them.

(the-ssh-key-operations)=
(the-ssh-key-declaration-rules)=
## The SSH key in the declaration layer

Keys are per-user records in the {ref}`controller <controller>`
database. The client's acts on them are:

- **Adding:** Keys are added verbatim: the material is parsed, its comment kept, its fingerprint taken with SHA-256, and the record stored under the user. A key the user already holds is refused.
  - *Rule:* Uniqueness is per user, twice over: the same material and the same fingerprint are each refused a second time for one user (the record's two unique indexes).
  - *Rule:* A key's material must parse as a public key.
  - *Rule:* The comment `juju-system-key` is reserved: the controller's own key carries it (see {ref}`the SSH key's machinery <the-ssh-keys-machinery>`), and a user key carrying that comment is refused.
  - *Rule:* The `ubuntu` account the keys land on is a member of the sudo group (the cloud config's own group list), so a key granted access to a machine grants its holder the machine's administrative shell.
  - *Related errors:*
    - **`public key already exists`**: *Trigger:* A key being added duplicates one the user already holds, by material or by fingerprint. *Remediation:* List the user's keys; the key is already stored.
    - **`invalid public key`**: *Trigger:* A key being added or imported does not parse as a public key. *Remediation:* Pass the complete public key line.
    - **`key contains a reserved comment`**: *Trigger:* A key's comment is the reserved controller key comment. *Remediation:* Add the key under a comment of your own.
- **Importing:** An import names a subject as a URI whose scheme selects the source (`gh:` for GitHub, `lp:` for Launchpad): the source's public keys for that subject are fetched and added under the importing user. Keys the user already holds are skipped, not errors.
  - *Rule:* An import source must be one the controller knows: the known schemes are `gh:` and `lp:`.
  - *Related errors:*
    - **`unknown import source`**: *Trigger:* An import subject's scheme has no resolver. *Remediation:* Import from GitHub or Launchpad.
    - **`import subject not found`**: *Trigger:* The import's source reports that the subject does not exist. *Remediation:* Check the subject name at the source.
- **Listing:** A listing returns a user's keys as full key material or as fingerprints (the stored SHA-256 ones).
- **Deleting:** A delete names targets: a fingerprint, a comment, or a full key value. Every match is removed; a target matching nothing is a no-op.

(the-ssh-key-in-the-data-model)=
(the-ssh-key-persistence-rules)=
## The SSH key in the persistence layer

The key records live in the controller database, projections included;
the machines' host keys live in each model database (see {ref}`the
database <database>`). An SSH key has no state machine:
it is added, listed, or deleted, nothing transitions.

The record set:

- **Fingerprint algorithm lookup:** The two values, `md5` and `sha256`.
  - *Rule:* The add and import acts fingerprint with SHA-256; the lookup's `md5` slot is unused by the current code paths.
- **User key record:** The material, its comment (also indexed for deletion, the schema's own comment), its fingerprint with the algorithm it was taken with, and the owning user. The key record's id is the join handle every projection points at.
  - *Rule:* Deleting a key removes its projections first; the key record itself goes only when no model projects it any more.
- **Projection record:** One record per (model, key) pair, a composite primary key; it carries no key material: it points at the key record. The pair is what a machine's authorisation read resolves.
  - *Rule:* The projection is the only model-facing record: a model authorises keys by pointing at records, never by copying material.
- **Machine host key records:** Records of their own, per model database: a UUID primary key, the {ref}`machine <machine>`, and the key material.
- **Authorisation view:** The authorisation read runs over a view that joins the projection to the user and the user's authentication.
  - *Rule:* A removed or disabled user's keys drop out of the authorisation read through the view, not through a rewrite.

(the-ssh-keys-machinery)=
## The SSH key in the execution layer

Nothing runs an SSH key. What runs is the projection onto machines and
the host-key reporting, split by owner:

- **The machine agents' key updater:** Each machine agent's key updater fetches the model's authorised keys through the agent facade, watches them, and rewrites the `ubuntu` account's `authorized_keys`: Juju-managed keys carry the Juju comment prefix, and keys already on the account that Juju does not manage are left alone.
- **The controller's own key:** Every machine's authorised-keys read also carries the controller's system key: generated at bootstrap, commented `juju-system-key`, its private half held by the controllers, its public half in the controller config. The controllers need no user key to reach a machine.
- **The host key reporting:** The machine agent reads its own host keys and reports them through the hostkeyreporter facade; the machine service replaces the machine's records. Bootstrap seeds the first host keys through the machine's cloud config. The client-facing ssh machinery reads the recorded host keys to verify the machine it connects to, gated on model admin access.
- **The model import:** A migrating model's keys import through the key manager's registered import operation: per-user keys come from the migration description; legacy model-config keys land under the admin user. Any key carrying the controller's key comment is dropped en route: the source controller must not keep access to the machines of a model it no longer owns (the import's own security note).
- **The unused surfaces:** The service exposes a delete-everything-for-a-model surface and an all-users grouped listing; neither has a non-test caller in the current code. The surfaces are stated, not narrated: nothing invokes them.

(the-ssh-key-watchers)=
### SSH key watchers

The user-facing key domain exposes no watch surface. The
machine-facing authorisation read is watchable: the key updater
service's per-machine notify watcher covers the model's projection
(the projection namespace) and the user-authentication
namespace, so a disabled user's keys ripple to the machines. Its one
consumer is the agent keyupdater facade, which the machine agents' key
updater drives.
