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

Use the {ref}`Juju CLI <juju-cli>` to manage {ref}`SSH public keys <ssh-key>` in a {ref}`model <model>`.

(add-an-ssh-key)=
## Add an SSH key

```{ibnote}
See also: {ref}`adding-an-ssh-key`
```

Sign in as the Juju user who will use the key. Pass the contents of that user's public `.pub` file, not the private key. For example, to register your Ed25519 key in `production` and check the result:

```text
juju add-ssh-key -m production "$(cat ~/.ssh/id_ed25519.pub)"
juju ssh-keys -m production
```

Compare the listed fingerprint with `ssh-keygen -lf ~/.ssh/id_ed25519.pub`. If it isn't listed, check the error from `add-ssh-key` before trying to connect.

```{ibnote}
See more: {ref}`command-juju-add-ssh-key`
```

(import-an-ssh-key)=
## Import an SSH key

```{ibnote}
See also: {ref}`importing-ssh-keys`
```

`juju import-ssh-key` imports into the Juju account named `admin`, **not** the account named on GitHub or Launchpad. If you want the key for a different Juju user, sign in as that user and {ref}`add the matching public key <add-an-ssh-key>` instead.

To import for `admin`, sign in as `admin`. Set `GITHUB_USER` to the GitHub account whose keys you intend to authorize, then specify the destination model:

```text
juju import-ssh-key -m production "gh:${GITHUB_USER:?set GITHUB_USER first}"
```

```{ibnote}
See more: {ref}`command-juju-import-ssh-key`
```

(view-the-available-ssh-keys)=
## View the available SSH keys

```{ibnote}
See also: {ref}`listing-ssh-keys`
```

To see your key fingerprints in the current model, run:

```text
juju ssh-keys
```

To inspect the full public keys in the `production` model:

```text
juju ssh-keys -m production --full
```

```{ibnote}
See more: {ref}`command-juju-ssh-keys`
```

(use-an-ssh-key)=
## Use an SSH key

```{ibnote}
See also: {ref}`ssh-server-access`
```

Before connecting, {ref}`register your public key <add-an-ssh-key>` for your Juju user in the destination model if it isn't registered there already. Select its matching **private** key with `--ssh-key`. For example, to open a shell on unit `mysql/0` in `production`:

```text
juju ssh -m production --ssh-key ~/.ssh/id_ed25519 mysql/0
```

```{ibnote}
See more: {ref}`command-juju-ssh`
```

````{dropdown} Example: Use a FIDO/U2F security key (e.g. YubiKey)
:color: success

If you installed Juju as a snap, connect the device interface before using the security key:

```text
sudo snap connect juju:u2f-devices
```

Generate a device-backed key, add its public key to the destination model, and select its private key for the connection:

```text
ssh-keygen -t ed25519-sk -f ~/.ssh/id_ed25519_sk
juju add-ssh-key -m production "$(cat ~/.ssh/id_ed25519_sk.pub)"
juju ssh -m production --ssh-key ~/.ssh/id_ed25519_sk mysql/0
```
````

(remove-an-ssh-key)=
## Remove an SSH key

```{ibnote}
See also: {ref}`removing-an-ssh-key`
```

To remove your laptop key from `production`, first compare the fingerprint of its public key file with your registered fingerprints **in that model**:

```text
ssh-keygen -lf ~/.ssh/id_ed25519.pub
juju ssh-keys -m production
```

If there is no exact match, stop. Otherwise set `FINGERPRINT` to the matching `SHA256:...` value. Remove it and check that it no longer appears in the same model:

```text
juju remove-ssh-key -m production "${FINGERPRINT:?set FINGERPRINT first}"
juju ssh-keys -m production
```

```{ibnote}
See more: {ref}`command-juju-remove-ssh-key`
```

