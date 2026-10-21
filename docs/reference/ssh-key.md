---
myst:
  html_meta:
    description: "SSH key reference: the public keys that users store on the controller, how a model authorizes them, the operations that add, import, list, and remove them, and the errors those raise."
---

(ssh-key)=
# SSH key

```{ibnote}
See also: {ref}`Juju | Manage SSH keys <manage-ssh-keys>`, {ref}`Terraform Provider for Juju | Manage SSH keys <tfjuju:manage-ssh-keys>`
```

An **SSH key** is a public key in the [SSH](https://www.ssh.com/academy/ssh-keys) protocol that a {ref}`user <user>` stores on a {ref}`controller <controller>`. A {ref}`model <model>` authorizes keys. Whoever holds the matching private key can then open an SSH session to the model's {ref}`machines <machine>` through the controller's {ref}`SSH server <juju-ssh>`.

```{warning}
A session opened through the controller lands as the `ubuntu` account on the machine, and on machines that Juju creates on a cloud that account has passwordless sudo, so authorizing a key gives its holder root access to the model's machines. On a {ref}`Kubernetes cloud <kubernetes-cloud>`, a session runs a shell in the {ref}`unit's <unit>` container through the Kubernetes API server. Authorize keys only for people who need root access to every machine in the model.
```

(ssh-key-database-schema)=
## SSH key database schema

```{ggarch}
:file: ../juju.ggarch
:view: SSH key database schema
:caption: The controller keeps one record for each SSH key that a user adds or imports. The record holds the key's text, its comment, and its fingerprint (a short code that identifies the key) with the algorithm that computed it, md5 or sha256, and it belongs to one user, who can have many keys but cannot store the same key, or two keys with the same fingerprint, twice. A key does nothing until a model authorizes it. Each authorization is its own small record that links one key to one model, so a key can be authorized on many models and a model can authorize many keys. The user and the model are described on their own pages, and the figure shows them only for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Five boxes of the same size, each with a filled table mark in the corner, and two of them (user and model) also with an outlined arrow mark for records that other pages describe. A user has many SSH keys (1 to m). Each SSH key names one fingerprint hash algorithm (m to 1) and is linked to models through model authorizations (1 to m). Each model has many model authorizations, so a key can be authorized on many models and a model can authorize many keys.
```

The SSH key database schema is the set of records that the {ref}`controller database <database>` keeps for SSH keys and their authorizations: one record per key, kept with its user, and one record per model that authorizes a key.

The controller keeps three kinds of record for SSH keys: the key itself, the algorithm that made its fingerprint, and the authorization that links a key to a model. Each has its own rules:

- **SSH key:** The key's owner, its comment, its public key text, and its fingerprint with the algorithm that produced it.
  - *Rules:*
    - A user stores the same key, or the same fingerprint, only once.
- **Fingerprint hash algorithm:** The allowed values are `md5` and `sha256`.
- **Model authorization:** Links one key to one model.
  - *Rules:*
    - A key is authorized on a model once.
    - The set of keys that a model honors leaves out the keys of disabled users. A disabled user's keys stay stored.
- **User and model:** The records on the two ends of a key's relations. Other pages own them: see {ref}`user <user>` and {ref}`model <model>`. The figure shows them for context. A key has one user. A model authorization names one model.

An SSH key is a row in the table `user_public_ssh_key`. The algorithm of its fingerprint comes from the lookup table `ssh_fingerprint_hash_algorithm`. Each model authorization is a row in `model_authorized_keys`. The view `v_model_authorized_keys` applies the rule about disabled users: query the view, not the table, to see the keys a model honors. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(ssh-key-operations)=
## SSH key operations

SSH key operations are the actions a {ref}`client <client>` can take on keys through the controller API (the set of calls that clients and agents make to the controller): add, import, list, and remove. Each acts on the model it addresses. The caller needs access to that model. Add, list, and remove act on the account that the client is logged in with. Import acts on the user that the request names. The keys it imports become the keys of that user.

(ssh-key-adding)=
### Adding SSH keys

```{ibnote}
See also: {ref}`Juju | Add an SSH key <add-an-ssh-key>`
```

Adding SSH keys stores one or more keys for the authenticated user and authorizes them on the model. The controller computes the fingerprint of each key with sha256.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller. The change block (an operator setting that makes the controller refuse operations that change the model; a refused operation fails with an "operation blocked" error that carries the operator's message) must not be on.
- *Rules:*
  - A key whose comment is `juju-system-key`, the comment of the key that the controller generates at {ref}`bootstrap <bootstrap-a-controller>` and keeps in the {ref}`controller configuration <controller-configuration>` key `system-ssh-keys` so that it can reach the machines it manages, is refused.
- *Errors:* The client receives each as its own error code with this message, where the user name is the one in the call:
  - **`one or more public keys to be added for user "<name>" already exist`**: *Trigger:* Adding a key that the model already authorizes. *Remediation:* Continue; the model already authorizes the key.
  - **`one or more public keys to be added for user "<name>" is invalid`**: *Trigger:* A key that is not a valid public key. *Remediation:* Provide the public half of the key, in the format of an `authorized_keys` line.
  - **`one or more public keys to be added for user "<name>" contains a restricted comment`**: *Trigger:* A key whose comment is the comment of the controller's own key. *Remediation:* Change the comment.
  - **`user "<name>" does not exist`** (code `user not found`): *Trigger:* The account that the client is logged in with does not exist. *Remediation:* Log in as an existing user.
  - **The operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Ask the operator to lift the block.

(ssh-key-importing)=
### Importing SSH keys

```{ibnote}
See also: {ref}`Juju | Import an SSH key <import-an-ssh-key>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Importing SSH keys
:caption: An import runs in this order: the controller checks the caller's access and the change block and finds the user, fetches the account's public keys from Launchpad or GitHub (a source that cannot be read as a URL fails first as an invalid source URL, a source whose prefix is neither `lp:` nor `gh:` fails here as an unknown source, and an account that does not exist fails here as a subject not found), parses each key and refuses an invalid key or a reserved comment, and only then stores the keys and authorizes them on the model, skipping any the model already authorizes. A failure before the store leaves that source's keys unstored, and the client gets one result for each source it named.
:alt: A sequence diagram with four participants: the client, the controller, Launchpad or GitHub, and the controller database. The client sends an import request to the controller. The controller checks access and the change block and finds the user, then asks Launchpad or GitHub for the account's public keys and receives them. The controller then parses each key and refuses a reserved comment, asks the controller database to store the keys and authorize them on the model, skipping any keys the model already authorizes, receives the confirmation, and returns one result for each source to the client.
```

Importing SSH keys fetches the keys of an account at an outside source and authorizes them on the model. The sources are [Launchpad](https://launchpad.net/) (`lp:<name>`) and [GitHub](https://github.com/) (`gh:<name>`).

The operation's required access, rules, and errors are:

- *Required access:* The same as adding. The change block must not be on. The access needed does not depend on whose keys are imported: a caller who may import keys can import them for any existing user. The keys are authorized on this model.
- *Rules:*
  - A key that carries the reserved comment of the controller's own key is refused.
  - An import skips keys that are already authorized on the model, without an error.
- *Errors:* The client receives each as its own error code with this message. The user errors come once for the call (a user removed during the call also fails a source with `user not found`). The others come once for each source it named:
  - **`invalid user name: "<name>"`** (code `invalid user name`) and **`user "<name>" does not exist`** (code `user not found`): *Trigger:* The user named in the call has an invalid name, or does not exist. *Remediation:* Name an existing user.
  - **`parsing key source url "<source>" for public key importing on user "<name>"`**: *Trigger:* A source that cannot be read as a URL. *Remediation:* Write the source as `lp:<name>` or `gh:<name>`.
  - **`cannot import from key source "<source>" for user "<name>", unknown source`**: *Trigger:* An import source whose prefix is neither `lp:` nor `gh:`. *Remediation:* Use one of the two prefixes.
  - **`cannot import from key source "<source>" for user "<name>", subject not found`**: *Trigger:* The source has no such account. *Remediation:* Check the spelling of the account.
  - **`one or more public keys to be imported from "<source>" for user "<name>" is invalid`**: *Trigger:* The source returned a key that is not a valid public key. *Remediation:* Fix the key at the source.
  - **`one or more public keys to be imported from "<source>" for user "<name>" contains a restricted comment`**: *Trigger:* The source returned a key whose comment is the comment of the controller's own key. *Remediation:* Change the comment at the source.

(ssh-key-listing)=
### Listing SSH keys

```{ibnote}
See also: {ref}`Juju | View the available SSH keys <view-the-available-ssh-keys>`
```

Listing SSH keys returns the authorized keys of the authenticated user on the model. A client can ask for the full key text or only the fingerprint. The caller needs {ref}`read access <user-access-model-read>` to the model (a higher level includes it).

The operation's errors are:

- *Errors:* The client receives each with its own error code:
  - **`invalid user name: "<name>"`** (code `invalid user name`) and **`user "<name>" does not exist`** (code `user not found`): *Trigger:* The account that the client is logged in with has an invalid name, or does not exist. *Remediation:* Log in as an existing user.

(ssh-key-removing)=
### Removing SSH keys

```{ibnote}
See also: {ref}`Juju | Remove an SSH key <remove-an-ssh-key>`
```

Removing SSH keys takes keys off the model, matched by fingerprint, public key text, or comment. A user's key that no model authorizes any more is deleted from the user's record too.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller, and neither the remove block (an operator setting that makes the controller refuse operations that remove things from the model, such as machines, applications, units, relations, and keys) nor the change block may be on.
- *Rules:*
  - A target that matches no key is ignored. The client gets no error.
  - Removing a user deletes the user's keys and their authorizations.
- *Errors:* The client receives each with its own error code:
  - **`user "<name>" does not exist`** (code `user not found`): *Trigger:* The account that the client is logged in with does not exist. *Remediation:* Log in as an existing user.
  - **The operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The change block or the remove block is on. *Remediation:* Ask the operator to lift the block.

## SSH keys and model migration

When a {ref}`model <model>` is migrated to another controller, its authorized keys move with it. The one exception is a model that arrives from an older controller, where the keys are part of the {ref}`model configuration <model-configuration>`: the import drops every key whose comment contains the reserved comment of a controller's own key, so that the controller the model came from keeps no SSH access to it. The remaining keys belong to the `admin` user.
