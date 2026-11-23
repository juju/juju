---
myst:
  html_meta:
    description: "SSH key reference: secure machine access in Juju with per-model public key management and ubuntu user account configuration."
---

(ssh-key)=
# SSH key

```{ibnote}
See also:

- {ref}`Juju CLI | Manage SSH keys <manage-ssh-keys>`
- {ref}`Terraform Provider for Juju | Manage SSH keys <tfjuju:manage-ssh-keys>`
```

An **SSH key** is an access  key in the [SSH](https://www.ssh.com/academy/ssh-keys) protocol. In Juju it refers to a way of accessing a machine provisioned by Juju individually.

Juju maintains a per-model cache of public SSH keys. You can add keys via `juju add-ssh-key` or `juju import-ssh-key`. Keys are used to authenticate users when they connect to the controller's SSH server.

Each Juju machine provides a user account named 'ubuntu' and this account is used when establishing SSH sessions. Because this user is effectively the 'root' user (passwordless sudo privileges), the granting of SSH access must be done with due consideration.

Your key must be registered for your user in the model you want to access. See {ref}`controller-ssh-server-targets` for the grants the controller checks and {ref}`controller-ssh-server-connections` for the two SSH connections.
