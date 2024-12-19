(ssh-key)=
# SSH key

> See also: {ref}`manage-ssh-keys`

<!--TODO: SEE IF WE NEED TO INCORPORATE THIS SOMEWHERE TOO:
### Model creators and SSH keys

When a controller is either created or registered a passphraseless SSH keypair will be generated and placed under `~/.local/share/juju/ssh`. The public key `juju_id_rsa.pub`, as well as a possibly existing `~/.ssh/id_rsa.pub`, will be placed within any newly-created model.

This means that a model creator will always be able to connect to any machine within that model (with `juju ssh`) without having to add keys since the creator is also granted 'admin' model access by default.
-->

An **SSH key** is an access  key in the [SSH](https://www.ssh.com/academy/ssh-keys) protocol. In Juju it refers to a way of accessing a machine provisioned by Juju individually.

Juju maintains a per-model cache of public SSH keys which it copies to each unit (including units already deployed). By default this includes the key of the user who created the model (assuming it is stored in the default location ~/.ssh/). You can also add further keys via `juju add-ssh-key` or `juju import-ssh-key`. Any key added to the model is placed on all machines (present and future) in the model.

Each Juju machine provides a user account named 'ubuntu' and it is to this account that public keys are added when using the Juju SSH commands `add-ssh-key` and `import-ssh-key`. Because this user is effectively the 'root' user (passwordless sudo privileges), the granting of SSH access must be done with due consideration.

To use an SSH key to run commands inside a machine using the `juju ssh` command, the user's public SSH key needs to be added to the containing model and the user needs to have `admin` access to the model.
