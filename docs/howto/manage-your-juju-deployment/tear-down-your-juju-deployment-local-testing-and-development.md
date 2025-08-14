(tear-things-down)=
# Tear down your deployment -- local testing and development

``````{tabs}
`````{group-tab} automatically
(tear-things-down-automatically)=

Delete the Multipass VM:

```text
$ multipass delete --purge my-juju-vm
```

Uninstall Multipass.

> See more: [Multipass | Uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall)

`````

`````{group-tab} manually
(tear-things-down-manually)=

1. Tear down Juju:

```text
# Destroy any models you've created:
$ juju destroy-model my-model

# Destroy any controllers you've created:
$ juju destroy-controller my-controller

# Uninstall juju. For example:
$ sudo snap remove juju
```

2. Tear down your cloud. E.g., for a MicroK8s cloud:

```text
# Reset Microk8s:
$ sudo microk8s reset

# Uninstall Microk8s:
$ sudo snap remove microk8s

# Remove your user from the snap_microk8s group:
$ sudo gpasswd -d $USER snap_microk8s
```

3. If earlier you decided to use Multipass, delete the Multipass VM:

```text
multipass delete --purge my-juju-vm
```

Then uninstall Multipass.

> See more: [Multipass | Uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall)

`````

``````