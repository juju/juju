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
## The SSH key in the declaration layer

Keys are per-user records in the {ref}`controller <controller>`
database. The client's acts on them are:

- **Adding:** keys are added verbatim: the material is parsed, its
  comment kept, its fingerprint taken with SHA-256, and the record
  stored under the user. A key the user already holds is refused.
- **Importing:** an import names a subject as a URI whose scheme
  selects the source (`gh:` for GitHub, `lp:` for Launchpad): the
  source's public keys for that subject are fetched and added under
  the importing user. Keys the user already holds are skipped, not
  errors.
- **Listing:** a listing returns a user's keys as full key material or
  as fingerprints (the stored SHA-256 ones).
- **Deleting:** a delete names targets: a fingerprint, a comment, or a
  full key value. Every match is removed; a target matching nothing is
  a no-op.

(the-ssh-key-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - Uniqueness is per user, twice over: the same material and the same
    fingerprint are each refused a second time for one user (the
    table's two unique indexes).
  - A key's material must parse as a public key.
  - The comment `juju-system-key` is reserved: the controller's own
    key carries it (see {ref}`the SSH key's machinery
    <the-ssh-keys-machinery>`), and a user key carrying that comment is
    refused.
  - An import source must be one the controller knows: the known
    schemes are `gh:` and `lp:`.
  - The `ubuntu` account the keys land on is a member of the sudo
    group (the cloud config's own group list), so a key granted
    access to a machine grants its holder the machine's
    administrative shell.
- **Errors:**
  - **`public key already exists`:** Triggered when a key being added
    duplicates one the user already holds, by material or by
    fingerprint. Remediation: list the user's keys; the key is
    already stored.
  - **`invalid public key`:** Triggered when a key being added or
    imported does not parse as a public key. Remediation: pass the
    complete public key line.
  - **`key contains a reserved comment`:** Triggered when a key's
    comment is the reserved controller key comment. Remediation: add
    the key under a comment of your own.
  - **`unknown import source`:** Triggered when an import subject's
    scheme has no resolver. Remediation: import from GitHub or
    Launchpad.
  - **`import subject not found`:** Triggered when the import's source
    reports the subject does not exist. Remediation: check the
    subject name at the source.

(the-ssh-key-in-the-data-model)=
## The SSH key in the persistence layer

The key records live in the controller database, projections included;
the machines' host keys live in each model database (see {ref}`the
full spine <data-model-full-spine>`). An SSH key has no state machine:
it is added, listed, or deleted, nothing transitions.

The record set over the DDL (`0021-user-ssh-keys.sql`,
`0022-model-authorized-keys.sql`, `0018-machine.sql`):

- **`ssh_fingerprint_hash_algorithm`:** the fingerprint algorithm
  lookup, `md5` and `sha256`.
- **`user_public_ssh_key`:** the user's key: the material, its
  comment (also indexed for deletion, the DDL's own comment), its
  fingerprint with the algorithm it was taken with, and the owning
  user.
- **`model_authorized_keys`:** the projection: one row per (model,
  key) pair, a composite primary key. The projection carries no key
  material: it points at the key row.
- **`machine_ssh_host_key`:** a machine's host keys, per model
  database: a UUID primary key, the {ref}`machine <machine>`, and the
  key material.
- **The view the authorisation read uses:**
  `v_model_authorized_keys` joins the projection to the user and the
  user's authentication: a removed or disabled user's keys stop being
  authorised (the view's own comment).

The identity: the key row's id is the join handle every projection
points at; the projection pair (model, key) is what a machine's
authorisation read resolves.

(the-ssh-key-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - The projection is the only model-facing record: a model authorises
    keys by pointing at rows, never by copying material.
  - Deleting a key removes its projections first; the key row itself
    goes only when no model projects it any more.
  - A removed or disabled user's keys drop out of the authorisation
    read through the view, not through a rewrite.
  - The add and import acts fingerprint with SHA-256; the algorithm
    lookup's `md5` slot is unused by the current code paths.

(the-ssh-keys-machinery)=
## The SSH key in the execution layer

Nothing runs an SSH key. What runs is the projection onto machines and
the host-key reporting, split by owner:

- **The machine agents' key updater:** each machine agent's key
  updater fetches the model's authorised keys through the agent
  facade, watches them, and rewrites the `ubuntu` account's
  `authorized_keys`: Juju-managed keys carry the Juju comment prefix,
  and keys already on the account that Juju does not manage are left
  alone.
- **The controller's own key:** every machine's authorised-keys read
  also carries the controller's system key: generated at bootstrap,
  commented `juju-system-key`, its private half held by the
  controllers, its public half in the controller config. The
  controllers need no user key to reach a machine.
- **The host key reporting:** the machine agent reads its own host
  keys and reports them through the hostkeyreporter facade; the
  machine service replaces the machine's rows. Bootstrap seeds the
  first host keys through the machine's cloud config. The
  client-facing ssh machinery reads the recorded host keys to verify
  the machine it connects to, gated on model admin access.
- **The model import:** a migrating model's keys import through the
  key manager's registered import operation: per-user keys come from
  the migration description; legacy model-config keys land under the
  admin user. Any key carrying the controller's key comment is
  dropped en route: the source controller must not keep access to the
  machines of a model it no longer owns (the import's own security
  note).
- **The unused surfaces:** the service exposes a
  delete-everything-for-a-model surface and an all-users grouped
  listing; neither has a non-test caller in the current code. The
  surfaces are stated, not narrated: nothing invokes them.

(the-ssh-key-watchers)=
### SSH key watchers

The user-facing key domain exposes no watch surface. The
machine-facing authorisation read is watchable: the key updater
service's per-machine notify watcher covers the model's projection
(the `model_authorized_keys` namespace) and the user-authentication
namespace, so a disabled user's keys ripple to the machines. Its one
consumer is the agent keyupdater facade, which the machine agents' key
updater drives.