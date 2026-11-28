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

Use `-c` to target the controller that holds a registration, and `-m` to target the model whose selection or revisions you want to inspect. The examples use a controller called `prod`, its model `payments`, and a Vault registration called `myvault`. Substitute your own names.

(configure-a-secret-backend)=
## Configure a secret backend

```{ibnote}
See also: {ref}`secret-backend-vault-settings`, {ref}`secret-backend-kubernetes-settings`
```

Start with a store that is already provisioned and accessible to Juju. Prepare a private YAML file containing the connection settings for **one** backend. For example, save this mapping as `vault_config.yaml`, substituting the HTTPS endpoint and an authorized token for your Vault:

```yaml
endpoint: https://vault.example.com:8200
token: "replace-with-your-vault-token"
```

Restrict access to the file. If your Vault uses a private CA, include the actual PEM contents as `ca-cert`, not a path. For a Kubernetes registration, prepare the Kubernetes settings instead, including the API endpoint and an existing namespace.

(add-a-secret-backend)=
## Add a secret backend

```{ibnote}
See also: {ref}`secret-backend-register`
```

As a superuser on `prod`, register the prepared Vault connection and check it by name:

```text
juju add-secret-backend -c prod myvault vault --config vault_config.yaml
juju show-secret-backend -c prod myvault
```

The add command prints nothing on success. The second command should return a `myvault` entry with `backend: vault`. No model has been switched by these commands.

```{ibnote}
See more: {ref}`command-juju-add-secret-backend`, {ref}`command-juju-show-secret-backend`
```

(list-secret-backends)=
## View all the secret backends available on a controller

```{ibnote}
See also: {ref}`secret-backend-inspection`
```

List the registrations on the controller:

```text
juju secret-backends -c prod
```

Read `Name` and `Type` to identify a registration. `Secrets` counts secrets with a revision referencing that backend, not models selecting it. For full messages and non-sensitive configuration, add `--format yaml`.

```{ibnote}
See more: {ref}`command-juju-secret-backends`
```

(inspect-a-secret-backend)=
## View details about a secret backend

```{ibnote}
See also: {ref}`secret-backend-inspection`
```

Inspect one name without exposing credentials:

```text
juju show-secret-backend -c prod myvault
```

If the command reports that no backends have been added, list the controller's backends and check the exact name. That message from a named lookup can mean just that **this name** did not match. If you need sensitive connection settings and are a controller superuser, use `--reveal` with named show or with list in YAML or JSON format. Keep the output private; it is backend configuration, not secret values.

```{ibnote}
See more: {ref}`command-juju-show-secret-backend`
```

(check-model-secret-backend)=
## Set or get the secret backend for a model

```{ibnote}
See also: {ref}`secret-backend-check-selection`, {ref}`secret-backend-select`
```

With at least read access to `payments`, check its selection:

```text
juju model-secret-backend -m prod:payments
```

Before selecting a backend, check {ref}`how it protects values at rest <secret-backend-encryption-at-rest>`. With at least write access to the model, select the registered name, then run the getter again:

```text
juju model-secret-backend -m prod:payments myvault
juju model-secret-backend -m prod:payments
```

The setter is silent on success; the getter should now print `myvault`. To return this model to its default, use `auto`, not `internal` or a name ending in `-local`:

```text
juju model-secret-backend -m prod:payments auto
juju model-secret-backend -m prod:payments
```

The getter should now print `auto`. That confirms the model's choice for new revisions. Before retiring the previous backend, {ref}`check where the existing revisions are stored <check-secret-backend-movement>`.

Use `model-secret-backend`, not the `secret-backend` key of `model-config`, on a 4.1 controller. If the dedicated command reports that an older controller does not support it, follow its compatibility instruction for that controller.

```{ibnote}
See more: {ref}`command-juju-model-secret-backend`
```

(check-secret-backend-movement)=
## Check where existing secret revisions are stored

```{ibnote}
See also: {ref}`secret-backend-movement`, {ref}`secret-revision-backend`
```

After selecting a different backend, inspect the affected model's revisions without revealing their values:

```text
juju secrets -m prod:payments --revisions --format yaml
```

For a single secret, use its name or ID instead:

```text
juju show-secret -m prod:payments db-password --revisions
```

Look at `backend` under each `revisions` entry. Repeat the read to follow a move; the revision number stays the same. When retiring `myvault`, also run `juju show-secret-backend -c prod myvault`: other models can account for references that remain in its `secrets` count. A zero count does not verify provider-side deletion.

```{ibnote}
See more: {ref}`command-juju-secrets`, {ref}`command-juju-show-secret`
```

(update-a-secret-backend)=
## Update a secret backend

```{ibnote}
See also: {ref}`secret-backend-update`, {ref}`secret-backend-rotation`
```

As a controller superuser, prepare a private file with the settings you want to change. For example, after updating the token in `vault_config.yaml`, apply it and request a two-hour token lifetime:

```text
juju update-secret-backend -c prod myvault --config vault_config.yaml token-rotate=2h
juju show-secret-backend -c prod myvault
```

Check the rotation interval in the result; sensitive settings remain hidden. To disable automatic token refresh for this Vault backend, use `token-rotate=0`. To restore a provider setting to its default, pass `--reset KEY` once for each key, not a comma-separated list. Only reset a credential if the remaining configuration still provides access. To rename a backend, supply `name=NEW` and use the new name for subsequent lookups.

```{ibnote}
See more: {ref}`command-juju-update-secret-backend`
```

(remove-a-secret-backend)=
## Remove a secret backend

```{ibnote}
See also: {ref}`secret-backend-remove`
```

Before retiring a user-added backend, choose the intended replacement for **each affected model**, then {ref}`check where its revisions are stored <check-secret-backend-movement>`. Keep the old store and credentials available during the move. As a controller superuser, once no references remain, request ordinary removal:

```text
juju remove-secret-backend -c prod myvault
juju secret-backends -c prod
```

Removal is silent on success. Check that `myvault` is absent from the inventory.

If Juju reports that the backend still contains secret content, check the name you tried to remove. For the {ref}`built-in backends <secret-backend-built-ins>` (`internal` and `kubernetes`), that message does not tell you whether they hold any values. Leave them in place. For a user-added backend, {ref}`check for remaining revision references <check-secret-backend-movement>` before retrying ordinary removal. Do not force removal to finish a move.

```{ibnote}
See more: {ref}`command-juju-remove-secret-backend`
```
