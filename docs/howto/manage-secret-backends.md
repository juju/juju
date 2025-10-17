(manage-secret-backends)=
# How to manage secret backends

```{ibnote}
See also: {ref}`secret-backend`
```

Starting with Juju `3.1.0`, you can also manage secret backends in a number of ways.

(configure-a-secret-backend)=
## Configure a secret backend

To configure a secret backend, create a configuration YAML file with configurations supported by your chosen backend type. Below we create a minimal configuration file  for a backend type `vault`, so we name the file `vault_config.yaml` and specify the API `endpoint` and the access `token`.

```{important}
Currently this is possible only for `vault`.
```

```{caution}
A minimal `vault` backend configuration as below is not secure. For production you should configure your `vault` backend securely by specifying further configuration keys, following the upstream Vault documentation.
```


```
cat > vault_config.yaml <<EOF
endpoint: http://10.0.0.1:8200
token: s.eujhj
EOF

```

That's it. You can now start using this backend by adding it to a model.

```{ibnote}
See more: {ref}`secret-backend-configuration-options`
```

<!--
```
juju add-secret-backend mysecrets vault \
--config=/path/to/vault_config.yaml \
token-rotate=7d
```
-->

(add-a-secret-backend)=
## Add a secret backend

Once you've configured a secret backend, to add it to a model, run the `add-secret-backend` command followed by your desired name and type for the backend, type as well as any relevant options:

```text
juju add-secret-backend myvault vault token-rotate=10m --config /path/to/cfg.yaml
```

```{ibnote}
See more: {ref}`command-juju-add-secret-backend`, {ref}`secret-backend`
```

## View all the secret backends available on a controller

To view all the backends available in the controller, run the `secret-backends` command:

```text
juju secret-backends
```

````{dropdown} Example output

```text
Backend           Type        Secrets  Message
internal          controller      134
foo-local         kubernetes       30
bar-local         kubernetes       30
myvault           vault            20  sealed
```

````

The command also has options that allow you to filter by a specific controller or set an output format or an output file or reveal sensitive backend config content.

```{ibnote}
See more: {ref}`command-juju-secret-backends`
```

## Set or get the secret backend for a model

**Set.** To set the secret backend to be used by a model, run the `model-secret-backend` command followed by the name of the desired secret backend. For example:

```text
juju model-secret-backend myVault
```

**Get.** To get the secret backend currently in use by a model, run the `model-secret-backend` command:

```text
juju model-secret-backend
```

```{ibnote}
See more: {ref}`command-juju-model-secret-backend`
```

## View details about a secret backend

To view details about a particular secret, use the `show-secret-backend` command followed by the name of the secret backend. For example, for a secret called `myvault`, do:

```text
juju show-secret-backend myvault
```

By passing various options you can also specify a controller, an output format, an output file, or whether to reveal sensitive information.

```{ibnote}
See more: {ref}`command-juju-show-secret-backend`
```

## Update a secret backend

To update a secret backend on the controller, run the `update-secret-backend` command followed by the name of the secret backend. Below we update the backend by supplying a configuration from a file:

```text
juju update-secret-backend myvault --config /path/to/cfg.yaml
```

```{ibnote}
See more: {ref}`command-juju-update-secret-backend`
```

## Remove a secret backend

To remove a secret backend, use the `remove-secret-backend` command followed by the backend name:

```text
juju remove-secret-backend myvault
```

```{ibnote}
See more: {ref}`command-juju-update-secret-backend`
```

