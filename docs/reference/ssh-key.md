---
myst:
  html_meta:
    description: "SSH key reference: secure machine access in Juju with per-model public key management and ubuntu user account configuration. The key record, operations, and rules."
---

(ssh-key)=
# SSH key
```{audience} user
```

```{ibnote}
See also: {ref}`manage-ssh-keys`
```

An **SSH key** is an access  key in the [SSH](https://www.ssh.com/academy/ssh-keys) protocol. In Juju it refers to a way of accessing a machine provisioned by Juju individually.

(the-ssh-keys-records)=
## The SSH key's records

### The SSH key's identity

In the **controller** database, an SSH key is a record owned by a
{ref}`user <user>`: the public key material, its fingerprint (with the
hash algorithm it was fingerprinted with -- MD5 or SHA-256), and a
comment. Uniqueness is per user: the same user cannot add the same
key or the same fingerprint twice. The keys reach the models through
a **projection** record -- one row per (model, key) pair -- which is
what each model's machines are told about.

(the-ssh-key-in-the-data-model)=
### The SSH key in the data model

Three record sets: the user's public keys (controller database), the
model projections naming which of them each model authorises, and --
in each model database -- the machines' own SSH **host keys** (the
keys the machines present, as opposed to the keys that access them).
The projection view excludes the keys of removed or disabled users.

(the-ssh-key-states)=
### SSH key states

An SSH key has no state machine: it is added, listed, or deleted --
nothing transitions.

(the-ssh-keys-machinery)=
## The SSH key's machinery

An SSH key has no machinery of its own: the controller stores the
records and projects them to the machines -- the key updater carries
each user's keys out as machines come up.

(the-ssh-key-operations)=
### SSH key operations

#### Adding and importing keys

Keys are added verbatim (`juju add-ssh-key`) or imported from a key
source (`juju import-ssh-key`, e.g. a Launchpad or GitHub user name);
the import resolves the source's public keys and stores them under
the importing user.

#### Projecting keys to machines

Juju maintains a per-model cache of public SSH keys which it copies to each machine (including machines already deployed). Any key added to the model is placed on all machines (present and future) in the model.

Each Juju machine provides a user account named 'ubuntu' and it is to this account that public keys are added when using the Juju SSH commands `add-ssh-key` and `import-ssh-key`. Because this user is effectively the 'root' user (passwordless sudo privileges), the granting of SSH access must be done with due consideration.

The machine agents keep this in step: each machine agent's key
updater worker fetches the model's authorised keys and rewrites the
`ubuntu` account's `authorized_keys`, prefixing Juju-managed keys
with a reserved comment and leaving non-Juju keys alone.

#### Deleting keys

Deleting a key removes it from the owner's set and from every
model's projection; the machines drop it on their next key update.

To use an SSH key to run commands inside a machine using the `juju ssh` command, the user's public SSH key needs to be added to the containing model and the user needs to have `admin` access to the model.

(the-ssh-key-watchers)=
### SSH key watchers
```{audience} juju-dev
```

The key domain exposes no watch surface of its own: the machine
agents' key updaters consume the API's key-update notifications, and
the projection -- which keys each model authorises -- is what they
re-fetch when it changes.

(the-ssh-key-rules-and-errors)=
## SSH key rules and errors
```{audience} charm-dev
```

- a user's keys are unique by fingerprint and by material -- the same
  key cannot be added twice under one user;
- the comment prefix Juju writes on its managed keys is reserved --
  a user key carrying that reserved comment is rejected (`reserved
  comment violation`);
- the key material must parse as a public key (`invalid public
  key`);
- an import needs a known source (`unknown import source`), and
  importing for an unknown subject fails (`import subject not
  found`).

(related-entities-ssh-key)=
## Entities related to the SSH key

- **Users** own the keys -- keys are per-user records, projected into
  models (see {ref}`user <user>`).
- **Machines** consume the projection: the `ubuntu` account's
  `authorized_keys` on every machine (see {ref}`machine <machine>`).
- **Models** are the scope of the projection -- one projection record
  per (model, key) pair (see {ref}`model <model>`).
