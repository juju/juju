---
myst:
  html_meta:
    description: "Manage SSH keys in Juju: add, list, import, and remove public keys for secure access to machines and controllers."
---

(manage-ssh-keys)=
# How to manage SSH keys

```{ibnote}
See also: {ref}`ssh-key`
```

If you've bootstrapped a controller, Juju has automatically created an SSH key
for you that yon use to SSH into the machines or units provisioned through
Juju. This document covers the other case where you want to add further SSH
keys to Juju.

(add-an-ssh-key)=
## Add an SSH key

To add a public `ssh` key to a model, use the `add-ssh-key` command followed by a string containing the entire key or an equivalent shell formula:

```text

# Use the entire ssh key:
juju add-ssh-key "ssh-rsa qYfS5LieM79HIOr535ret6xy
AAAAB3NzaC1yc2EAAAADAQA6fgBAAABAQCygc6Rc9XgHdhQqTJ
Wsoj+I3xGrOtk21xYtKijnhkGqItAHmrE5+VH6PY1rVIUXhpTg
pSkJsHLmhE29OhIpt6yr8vQSOChqYfS5LieM79HIOJEgJEzIqC
52rCYXLvr/BVkd6yr4IoM1vpb/n6u9o8v1a0VUGfc/J6tQAcPR
ExzjZUVsfjj8HdLtcFq4JLYC41miiJtHw4b3qYu7qm3vh4eCiK
1LqLncXnBCJfjj0pADXaL5OQ9dmD3aCbi8KFyOEs3UumPosgmh
VCAfjjHObWHwNQ/ZU2KrX1/lv/+lBChx2tJliqQpyYMiA3nrtS
jfqQgZfjVF5vz8LESQbGc6+vLcXZ9KQpuYDt joe@ubuntu"


# Use an equivalent shell formula:
juju add-ssh-key "$(cat ~/mykey.pub)"

```

```{ibnote}
See more: {ref}`command-juju-add-ssh-key`
```

## Import an SSH key

To import a public SSH key from Launchpad / Github to a model, use the `import-ssh-key` command followed by `lp:` / `gh:` and the name of the user account. For example, the code below imports all the public keys associated with the Github user account ‘phamilton’:

```text
juju import-ssh-key gh:phamilton
```

```{ibnote}
See more: {ref}`command-juju-import-ssh-key`
```

## View the available SSH keys

To list the SSH keys known in the current model, use the `ssh-keys` command.

```text
juju ssh-keys
```

If you want to get more details, or get this information for a different model, use the `--full` or the `--model / -m <model name>` option.

```{ibnote}
See more: {ref}`command-juju-ssh-keys`
```

(use-an-ssh-key)=
## Use an SSH key

To SSH into a machine using a specific private key, pass OpenSSH's `-i`
flag between the target and a possible remote command. Because `juju ssh`
passes any options placed after the target to the underlying OpenSSH client,
other OpenSSH flags can be used in the same way:

```text
juju ssh ubuntu/0 -i ~/.ssh/my_private_key
```

The key's public counterpart must be added to the model first (see
{ref}`add-an-ssh-key`).

```{ibnote}
See more: {ref}`command-juju-ssh`
```

````{dropdown} Example: Use a FIDO/U2F security key (e.g. YubiKey)
:color: success

To use a FIDO/U2F security key with `juju ssh`, generate an SSH key
backed by the security key, add the public key to the model, and pass
the private key with the `-i` option:

```text
ssh-keygen -t ed25519-sk -f ~/.ssh/id_ed25519_sk
juju add-ssh-key "$(cat ~/.ssh/id_ed25519_sk.pub)"
juju ssh ubuntu/0 -i ~/.ssh/id_ed25519_sk
```

When using the Juju snap, the `u2f-devices` interface must be connected
to allow access to FIDO/U2F security keys. This interface is not
auto-connected:

```text
sudo snap connect juju:u2f-devices
```
````

## Remove an SSH key

To remove an SSH key, use the `remove-ssh-key` command followed by the key / a space-separated list of keys. The keys may be specified by either their fingerprint or the text label associated with them. The example below illustrates both:

```text
juju remove-ssh-key 45:7f:33:2c:10:4e:6c:14:e3:a1:a4:c8:b2:e1:34:b4 bob@ubuntu
```

```{ibnote}
See more: {ref}`command-juju-remove-ssh-key`
```

