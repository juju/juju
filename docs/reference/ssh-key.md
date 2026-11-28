---
myst:
  html_meta:
    description: "Public SSH keys in Juju: model authorization, ownership, and the requirements for adding, importing, listing, and removing keys."
---

(ssh-key)=
# SSH key

```{ibnote}
See also: {ref}`Juju CLI | Manage SSH keys <manage-ssh-keys>`, {ref}`Terraform Provider for Juju | Manage SSH keys <tfjuju:manage-ssh-keys>`
```

```{ggarch}
:file: ../juju.ggarch
:view: SSH key authorizations
:caption: Alice has registered the same laptop key in both models. Removing it from production leaves her staging registration and Ben's production registration unchanged.
:alt: Two models with their registered public keys. Staging lists Alice's laptop key. Production lists Alice's laptop key and Ben's workstation key.
```

In Juju, an **SSH key** is a [public key](https://www.rfc-editor.org/rfc/rfc4252#section-7) that Juju registers for a {ref}`user <user>` in a particular {ref}`model <model>`. For example, when you run {ref}`juju ssh <command-juju-ssh>` with its default settings, your client connects to the {ref}`controller's SSH server <ssh-server-connections>`. Before opening a session, the server checks both that {ref}`your key is registered in the destination model <ssh-key-authorization>` and that {ref}`your user has SSH access to that model <ssh-server-access>`.

(ssh-key-authorization)=
## SSH key authorization

A user can register the same public key in more than one model. Registering it in one model authorizes it only there.

At bootstrap, Juju registers any public keys supplied for the controller model under its `admin` user.

(adding-an-ssh-key)=
## Adding an SSH key

```{ibnote}
See also: {ref}`Juju CLI | Add an SSH key <add-an-ssh-key>`, [Terraform Provider for Juju | Add an SSH key](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-ssh-keys/#add-an-ssh-key)
```

Adding a key authorizes it for the caller in the selected model. Each entry must contain one public key. Juju adds the keys together: if it refuses any entry, it adds none of them.

**Who can do this:** A {ref}`model admin <user-access-model-admin>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `one or more public keys to be added for user "<user>" is invalid`. *Trigger:* An entry does not parse as one public key. *What to do:* Supply each complete public key as a separate entry.
- *Error message:* `one or more public keys to be added for user "<user>" contains a restricted comment`. *Trigger:* A key has the reserved comment `juju-system-key`. *What to do:* Change or omit that comment.
- *Error message:* `one or more public keys to be added for user "<user>" already exist`. *Trigger:* A fingerprint is already authorized for the caller in this model or occurs twice in the request. *What to do:* {ref}`List the fingerprints <listing-ssh-keys>` and submit only new keys, once each.
- *Error message:* `permission denied`. *Trigger:* The caller lacks permission to add keys. *What to do:* Request model `admin` access for the user who needs the key.
- *Error message:* The message supplied when changes were blocked, or `the operation has been blocked` if none was supplied. *Trigger:* Changes to the model are blocked. *What to do:* Ask the model administrator about lifting the block.
- *Error message:* `user "<user>" does not exist`. *Trigger:* Juju cannot find the authenticated user's account. *What to do:* Check which Juju account is signed in on the controller.

(importing-ssh-keys)=
## Importing SSH keys

```{ibnote}
See also: {ref}`Juju CLI | Import SSH keys <import-an-ssh-key>`, [Terraform Provider for Juju | Add keys from GitHub or Launchpad](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-ssh-keys/#add-an-ssh-key)
```

An import authorizes public keys fetched from an external account for a Juju user in the selected model. Juju skips keys already authorized for that user. Later changes to the external account do not update the model.

Juju processes each source separately. A failure at one source does not undo a successful import from another.

**Who can do this:** A {ref}`model admin <user-access-model-admin>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `parsing key source url "<source>" for public key importing on user "<user>"`. *Trigger:* The source is not a valid URI. *What to do:* Supply a source such as `gh:<account>` or `lp:<account>`.
- *Error message:* `cannot import from key source "<source>" for user "<user>", unknown source`. *Trigger:* Juju has no importer for that source. *What to do:* Select GitHub (`gh:`) or Launchpad (`lp:`).
- *Error message:* ends with `subject cannot be empty`. *Trigger:* The source names a service but no account. *What to do:* Include the external account name after the colon.
- *Error message:* `cannot import from key source "<source>" for user "<user>", subject not found`. *Trigger:* The external service did not find the account. *What to do:* Check the account name on that service, not the Juju username.
- *Error message:* `one or more public keys to be imported from "<source>" for user "<user>" is invalid`. *Trigger:* The source returned an entry that does not parse as one public key. *What to do:* Correct the entry at the source before importing again.
- *Error message:* `one or more public keys to be imported from "<source>" for user "<user>" contains a restricted comment`. *Trigger:* An imported key has the reserved comment `juju-system-key`. *What to do:* Change or omit the comment at the source before importing again.
- *Error message:* `invalid user name: "<user>"` or `user "<user>" does not exist`. *Trigger:* The destination Juju username is invalid or the account does not exist on the controller. *What to do:* Check the Juju account meant to receive the keys.
- *Error message:* `permission denied`. *Trigger:* The caller lacks permission to import keys. *What to do:* If the keys are for your own Juju account, request `admin` access to the destination model and {ref}`add the public key while signed in as yourself <add-an-ssh-key>`. The {ref}`Juju CLI import command <import-an-ssh-key>` always names the Juju user `admin` as the recipient; it cannot import keys for a different user.
- *Error message:* The message supplied when changes were blocked, or `the operation has been blocked` if none was supplied. *Trigger:* Changes to the model are blocked. *What to do:* Ask the model administrator about lifting the block.

(listing-ssh-keys)=
## Listing SSH keys

```{ibnote}
See also: {ref}`Juju CLI | List SSH keys <view-the-available-ssh-keys>`, [Terraform Provider for Juju | Query SSH keys in a model](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-models/#define-what-you-want-to-query-for)
```

The list contains only the caller's keys in the selected model, even when the caller administers the model. A user can request SHA256 fingerprints or the full public keys. Both forms return the same set: to inspect one key, match its fingerprint in the list. A fingerprint identifies the key independently of its comment. If the caller has no keys in the model, the list is empty; this is not an error.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` to the model, or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `permission denied`, or `access permissions required: …` for token-based access. *Trigger:* The caller lacks permission to read the model. *What to do:* Request model `read` access, or a token that includes it.
- *Error message:* `user "<user>" does not exist`. *Trigger:* Juju cannot find the authenticated user's account. *What to do:* Check which Juju account is signed in on the controller.

(removing-an-ssh-key)=
## Removing an SSH key

```{ibnote}
See also: {ref}`Juju CLI | Remove an SSH key <remove-an-ssh-key>`, [Terraform Provider for Juju | Remove an SSH key](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-ssh-keys/#remove-an-ssh-key)
```

Removal withdraws the caller's key authorization in the selected model. The same key remains authorized in any other model where the user registered it.

Juju matches the exact fingerprint, full public key, or comment. A comment shared by several of the caller's keys matches all of them; an identifier that matches nothing leaves the model unchanged and does not produce an error.

**Who can do this:** A {ref}`model admin <user-access-model-admin>` or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `permission denied`. *Trigger:* The caller lacks permission to remove keys. *What to do:* Request model `admin` access for the user whose keys these are.
- *Error message:* The message supplied when removals or changes were blocked, or `the operation has been blocked` if none was supplied. *Trigger:* Removals or changes are blocked on the model. *What to do:* Ask the model administrator about lifting the block.
- *Error message:* `user "<user>" does not exist`. *Trigger:* Juju cannot find the authenticated user's account. *What to do:* Check which Juju account is signed in on the controller.
