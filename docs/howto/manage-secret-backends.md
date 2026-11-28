---
myst:
  html_meta:
    description: "Register and inspect a secret backend, select it for a model, follow revision movement, and retire the old registration."
---

(manage-secret-backends)=
# How to manage secret backends

```{ibnote}
See also: {ref}`secret-backend`
```

Use the {ref}`Juju CLI <juju-cli>` to register a secret backend on a controller and choose which backend a model uses.

(configure-a-secret-backend)=
## Configure a secret backend

```{ibnote}
See also: {ref}`secret-backend-vault-settings`, {ref}`secret-backend-kubernetes-settings`
```

To connect Juju to an existing store, put its connection settings in a YAML file. Make the file readable only by your account before adding credentials. For example, for a Vault store, put its HTTPS endpoint and an authorized token in `vault_config.yaml`:

```yaml
endpoint: https://vault.example.com:8200
token: "replace-with-your-vault-token"
```

If your Vault uses a private CA, put the CA certificate's PEM contents in `ca-cert`, not the path to the certificate. For a Kubernetes store, use the {ref}`Kubernetes connection settings <secret-backend-kubernetes-settings>` instead: specify the API endpoint, an existing namespace, and the credentials and CA data your cluster requires.

(add-a-secret-backend)=
## Add a secret backend

```{ibnote}
See also: {ref}`secret-backend-register`
```

To register a store, run `juju add-secret-backend` with a name for the registration, the provider type, and the {ref}`connection settings file <configure-a-secret-backend>`. For example, as a controller superuser, register a Vault store as `myvault` on controller `prod` and inspect it:

```text
juju add-secret-backend -c prod myvault vault --config vault_config.yaml
juju show-secret-backend -c prod myvault
```

The add command prints nothing on success. Look for `backend: vault` under `myvault` in the second command's output. To use it for new revisions, {ref}`select it for a model <check-model-secret-backend>`.

```{ibnote}
See more: {ref}`command-juju-add-secret-backend`, {ref}`command-juju-show-secret-backend`
```

(list-secret-backends)=
## View all the secret backends available on a controller

```{ibnote}
See also: {ref}`secret-backend-inspection`
```

To list the backends on a controller, run `juju secret-backends`. For example, to list those on `prod`:

```text
juju secret-backends -c prod
```

Find the registration in the `Name` column and its provider in `Type`. To see full status messages and non-sensitive settings, add `--format yaml`.

```{ibnote}
See more: {ref}`command-juju-secret-backends`
```

(inspect-a-secret-backend)=
## View details about a secret backend

```{ibnote}
See also: {ref}`secret-backend-inspection`
```

To inspect one registration, give its name to `juju show-secret-backend`. For example, to check `myvault` on controller `prod` without showing credentials:

```text
juju show-secret-backend -c prod myvault
```

If the command says no backends have been added, {ref}`list the controller's backends <list-secret-backends>` and check the spelling. The message can mean that only the requested name was missing.

If you need sensitive connection settings, run the same command with `--reveal` as a controller superuser. Keep the output private. It contains backend settings, not the secret values held in the store.

```{ibnote}
See more: {ref}`command-juju-show-secret-backend`
```

(check-model-secret-backend)=
## Set or get the secret backend for a model

```{ibnote}
See also: {ref}`secret-backend-check-selection`, {ref}`secret-backend-select`
```

To check which backend a model uses for new revisions, run `juju model-secret-backend` without a backend name. For example, with read access to model `payments` on controller `prod`, run:

```text
juju model-secret-backend -m prod:payments
```

To switch the model to a different backend, first check {ref}`how that backend protects values at rest <secret-backend-encryption-at-rest>`. With write access to the model, pass the registration name, then check the selection. For example, to select `myvault`:

```text
juju model-secret-backend -m prod:payments myvault
juju model-secret-backend -m prod:payments
```

The first command prints nothing on success; the second should print `myvault`. If you want the platform default instead, use `auto`:

```text
juju model-secret-backend -m prod:payments auto
juju model-secret-backend -m prod:payments
```

The second command should now print `auto`. That confirms the choice for new revisions. Before retiring the previous backend, {ref}`check where the existing revisions are stored <check-secret-backend-movement>`.

Use `juju model-config` for this task only if an older controller reports that `model-secret-backend` is not implemented and directs you to `model-config`.

```{ibnote}
See more: {ref}`command-juju-model-secret-backend`
```

(check-secret-backend-movement)=
## Check where existing secret revisions are stored

```{ibnote}
See also: {ref}`secret-backend-movement`, {ref}`secret-revision-backend`
```

To follow a backend change, list revisions in the affected model without revealing their values. For example, for model `payments` on controller `prod`, run:

```text
juju secrets -m prod:payments --revisions --format yaml
```

To check just one secret, give its name or ID to `juju show-secret`. For example:

```text
juju show-secret -m prod:payments db-password --revisions
```

Check the `backend` field of each revision you expect to move. Repeat the command until those revisions show the intended backend.

If you plan to retire the old backend, also check its count across the controller. For example, if `myvault` is the old backend on `prod`, run:

```text
juju show-secret-backend -c prod myvault
```

If `secrets` is nonzero, revisions still reference `myvault`, possibly in another model. A zero count does not prove that old copies were deleted from the store.

```{ibnote}
See more: {ref}`command-juju-secrets`, {ref}`command-juju-show-secret`
```

(update-a-secret-backend)=
## Update a secret backend

```{ibnote}
See also: {ref}`secret-backend-update`, {ref}`secret-backend-rotation`
```

To change a registration's settings, give `juju update-secret-backend` its current name and the settings to change. For example, as a superuser on controller `prod`, use `--config` to supply a new Vault token from a private YAML file and `token-rotate` to request a two-hour token lifetime for `myvault`:

```text
juju update-secret-backend -c prod myvault --config vault_config.yaml token-rotate=2h
juju show-secret-backend -c prod myvault
```

Check `token-rotate-interval` in the output of `juju show-secret-backend`. To turn off token refresh, run the update with `token-rotate=0`.

To reset several provider settings, pass `--reset KEY` once per key, not a comma-separated list. Reset a credential only if the remaining configuration still provides access. To rename the registration, pass `name=NEW` and use the new name when you inspect it.

```{ibnote}
See more: {ref}`command-juju-update-secret-backend`
```

(remove-a-secret-backend)=
## Remove a secret backend

```{ibnote}
See also: {ref}`secret-backend-remove`
```

To retire a backend you added:

1. In each model that uses it, {ref}`select the replacement backend <check-model-secret-backend>`.
2. Keep the old store and its credentials available while you {ref}`check revision locations and the old backend's controller-wide count <check-secret-backend-movement>`. Wait until the count is zero.
3. As a controller superuser, remove the registration and list the backends to confirm its name is gone. For example, to remove `myvault` from `prod`:

   ```text
   juju remove-secret-backend -c prod myvault
   juju secret-backends -c prod
   ```

If Juju reports that the backend still contains secret content, check the name you tried to remove. For the {ref}`built-in backends <secret-backend-built-ins>` (`internal` and `kubernetes`), that message does not tell you whether they hold any values. Leave them in place. For a user-added backend, {ref}`check for remaining revision references <check-secret-backend-movement>` before retrying ordinary removal. Do not force removal to finish a move.

```{ibnote}
See more: {ref}`command-juju-remove-secret-backend`
```
