---
myst:
  html_meta:
    description: "SSH key reference: the user public keys the controller's SSH server checks and the machines' own host keys. The records, the acts on keys, and the SSH server, reverse tunnel and host key reporting machinery."
---

(ssh-key)=
# SSH key

```{ibnote}
See also: {ref}`manage-ssh-keys`
```

An **SSH key** gives its holder shell access to a machine Juju
provisioned. The records come in two families: the {ref}`user <user>`'s
public keys, which the controller checks when the user connects through its
SSH server, and the machines' own SSH **host keys**, the keys the machines
and the controller's SSH server present to whoever connects.

Juju keeps each user's public SSH keys in the controller and associates them
with models. You add keys with `juju add-ssh-key` or `juju import-ssh-key`.
Keys authenticate users when they connect to the controller's SSH server.

Each Juju machine provides a user account named `ubuntu`, and the SSH
sessions Juju establishes use this account. Because this user is effectively
the `root` user (passwordless sudo privileges), grant SSH access with due
consideration.

To run commands inside a machine with `juju ssh`, add the user's public SSH
key to the containing model; the user also needs `admin` access to the model.

See {ref}`juju-ssh` for a deeper dive into how SSH connections are
established through the Juju controller.

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

The user key records live in the controller database, associations with
models included, together with the controller's own host key and the SSH
server port; the machines' host keys, the virtual host keys and the tunnel
requests live in each model database (see {ref}`the database <database>`).
An SSH key has no state machine: it is added, listed, or deleted, nothing
transitions.

The record set:

- **Fingerprint algorithm lookup:** The two values, `md5` and `sha256`.
  - *Rule:* The add and import acts fingerprint with SHA-256; the lookup's `md5` slot is unused by the current code paths.
- **User key record:** The material, its comment (also indexed for deletion, the schema's own comment), its fingerprint with the algorithm it was taken with, and the owning user. The key record's id is the join handle every association points at.
  - *Rule:* Deleting a key removes its model associations first; the key record itself goes only when no model carries it any more.
- **Model association record:** One record per (model, key) pair, a composite primary key; it carries no key material: it points at the key record. The pair is what the server's "is this key associated with the model" check resolves.
  - *Rule:* The association is the only model-facing record: a model authorises keys by pointing at records and holds no copy of the material.
- **Machine host key records:** Records of their own, per model database: a UUID primary key, the {ref}`machine <machine>`, and the key material.
- **Virtual host key records:** One per machine and one per Kubernetes unit in the model database, each holding the key's algorithm and material. The terminating SSH server generates one on first use.
- **Controller host key and server port:** A singleton controller host key record (algorithm, key and public half) and a singleton server port record; the server worker watches the port and restarts on a change.
- **Connection request record:** One per pending tunnel, keyed by the tunnel ID, naming the {ref}`machine <machine>`, an expiry, the controller addresses, the unit port, and the ephemeral public key.
- **Authorisation view:** The view of the keys authorised on a model joins the association to the user and the user's authentication.
  - *Rule:* A removed or disabled user's keys drop out of the view as their state changes; no rewrite of the association is needed.

(the-ssh-keys-machinery)=
## The SSH key in the execution layer

Nothing runs an SSH key. What runs is the controller's SSH server, which
checks keys when a user connects, and the machine side of the reverse
tunnel, which carries the connection to the machine:

- **The controller's SSH server:** A user's key authenticates at the controller's SSH server. The server reads the keys registered for the connecting user and accepts the connection when the presented key matches one of them. It then authorises the destination in two steps: the user needs SSH access to the model, and the key used needs to be associated with that model (the check compares SHA-256 fingerprints). A user holding several keys who presents one the model does not carry is refused, with a message naming the key's comment.
  - *Rule:* The server fails closed: authorisation needs a connection that authenticated with a public key.
  - *Related errors:*
    - **`public key %q used to authenticate is not associated with model %q, add the key to the model or specify a different key`**: *Trigger:* The key a user connected with is registered for the user but not associated with the destination model. *Remediation:* Add the key to the model or connect with a different key.
- **The reverse tunnel:** Machines can reach the controller but the controller cannot reach machines, so a connection to a machine is a one-shot request. The controller records an SSH connection request, keyed by a tunnel ID and carrying an ephemeral public key, in the model database. The machine agent's SSH session worker watches the requests and, for a request naming its own machine, reverse-dials the controller and pipes the tunnel to the machine's local `sshd`.
- **The machine agent's key updater:** The key updater worker in the machine agent only serialises the ephemeral keys of tunnels. The SSH session worker has it add the request's ephemeral key to the `ubuntu` account's `authorized_keys` (commented with the Juju ephemeral prefix and the tunnel ID) before dialling, and remove it when the tunnel ends. User keys stay at the controller, and keys on the account that Juju does not manage are left alone.
- **The keys a machine starts with:** At provisioning, the machine's cloud config seeds the `ubuntu` account's authorised keys. The `keyupdater` facade, which serves a machine's authorised keys (the model's user keys plus the controller's system key) and a watcher over them, is kept registered for older agents during rolling upgrades and migrations from 3.6; current agents do not consume it.
- **The controller's own key:** The controller's system key is generated at bootstrap and commented `juju-system-key`, with its public half in the controller config. The facade above includes it in a machine's authorised keys.
- **The host keys:** The machine agent reads its own host keys and reports them through the hostkeyreporter facade; the machine service replaces the machine's records. The tunnel side reads the recorded host keys to verify the machine it connects to. The terminating SSH server presents a virtual host key instead: it is generated and persisted on first use, once per machine (an IAAS unit shares its machine's key, a Kubernetes unit has its own), and a nested machine is refused with advice to connect to its parent.
  - *Related errors:*
    - **`cannot SSH directly to nested machine %q, connect to parent machine %q instead`**: *Trigger:* The destination is a container machine. *Remediation:* Connect to the parent machine.
- **The model import:** A migrating model's keys import through the key manager's registered import operation: per-user keys come from the migration description; legacy model-config keys land under the admin user. Any key carrying the controller's key comment is dropped en route: the source controller must not keep access to the machines of a model it no longer owns (the import's own security note).
- **The unused surfaces:** The service exposes a delete-everything-for-a-model surface and an all-users grouped listing; neither has a non-test caller in the current code. The surfaces are stated, not narrated: nothing invokes them.

(the-ssh-key-watchers)=
### SSH key watchers

The user-facing key domain exposes no watch surface. Three notify and
strings watchers serve the SSH machinery:

- **The SSH server port:** The controller SSH service watches the server's configured port.
- **The connection requests:** The model SSH service watches the pending connection requests for a machine, so the machine agent's SSH session worker reacts to a new tunnel request.
- **The host keys:** The machine service watches a machine's recorded host keys for the tunnel side.

The key updater service's per-machine watcher over the model's key associations
and the user-authentication namespaces remains behind the compatibility
`keyupdater` facade. No current agent consumes it.
