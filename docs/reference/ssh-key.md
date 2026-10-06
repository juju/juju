---
myst:
  html_meta:
    description: "SSH key reference: secure machine access in Juju with per-model public key management and ubuntu user account configuration."
---

(ssh-key)=
# SSH key

```{ibnote}
See also: {ref}`manage-ssh-keys`
```

An **SSH key** is an access  key in the [SSH](https://www.ssh.com/academy/ssh-keys) protocol. In Juju it refers to a way of accessing a machine provisioned by Juju individually.

Juju maintains a per-model cache of public SSH keys. You can add keys via `juju add-ssh-key` or `juju import-ssh-key`. Keys are used to authenticate users when they connect to the controller's SSH server.

Each Juju machine provides a user account named 'ubuntu' and this account is used when establishing SSH sessions. Because this user is effectively the 'root' user (passwordless sudo privileges), the granting of SSH access must be done with due consideration.

To use an SSH key to run commands inside a machine using the `juju ssh` command, the user's public SSH key needs to be added to the containing model and the user needs to have `admin` access to the model.

See {ref}`explanation-juju-ssh` for a deeper dive into how SSH connections are established through the Juju controller.
