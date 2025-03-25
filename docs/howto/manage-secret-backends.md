(manage-secret-backends)=
# How to manage secret backends

> See also: {ref}`secret-backend`

Starting with Juju `3.1.0`, you can also manage secret backends in a number of ways.


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

> See more: {ref}`secret-backend-configuration-options`

<!--
```
juju add-secret-backend mysecrets vault \
--config=/path/to/vault_config.yaml \
token-rotate=7d
```
-->

## Add a secret backend to a model



To add a secret backend to a model, run the `add-secret-backend` command followed by your desired name and type for the backend, type as well as any relevant options:

```text
juju add-secret-backend myvault vault token-rotate=10m --config /path/to/cfg.yaml
```

> See more: {ref}`command-juju-add-secret-backend`, {ref}`secret-backend`


## View all the secret backends available on a controller

To view all the backends available in the controller, run the `secret-backends` command:

```text
juju secret-backends
```

````{dropdown} Expand to see a sample output

```text
Backend           Type        Secrets  Message
internal          controller      134  
foo-local         kubernetes       30
bar-local         kubernetes       30
myvault           vault            20  sealed
```

````

The command also has options that allow you to filter by a specific controller or set an output format or an output file or reveal sensitive backend config content.

> See more: {ref}`command-juju-secret-backends`


## View all the secret backends active in a model

To see all the secret backends in use on a model, use the `show-model` command. Beginning with Juju `3.1`, this command also shows the secret backends (though you might have to scroll down to the end).

```text
juju show-model
```

````{dropdown} Expand to see a sample output

```text
mymodel:
  name: admin/mymodel
  short-name: mymodel
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  model-type: iaas
  controller-uuid: deadbeef-1bad-500d-9000-4b1d0d06f00d
  controller-name: kontroll
  owner: admin
  cloud: aws
  region: us-east-1
  type: ec2
  life: alive
  status:
	current: available
  users:
	admin:
  	display-name: admin
  	access: admin
  	last-connection: just now
  machines:
	"0":
  	  cores: 0
	"1":
  	  cores: 2
  secret-backends:
	myothersecrets:
  	  status: active
	  secrets: 6
	mysecrets:
  	  status:draining
	  secrets: 5
```

````

> See more: {ref}`command-juju-show-model`

## Change the secret backend to be used by a model

To change the secret backend to be used by a model, use the `model-config` command with the `secret-backend` key configured to the name of the secret backend that you want to use, for example, `myothersecrets`:

```text
juju model-config secret-backend=myothersecrets
```

After the switch, any new secret revisions are stored in the new backend. Existing revisions continue to be read from the old backend.

> See more: {ref}`configure-a-model`, {ref}`model-config-secret-backend`

## View details about a secret backend

To view details about a particular secret, use the `show-secret-backend` command followed by the name of the secret backend. For example, for a secret called `myvault`, do:

```text
juju show-secret-backend myvault
```

By passing various options you can also specify a controller, an output format, an output file, or whether to reveal sensitive information. 

> See more: {ref}`command-juju-show-secret-backend`

## Update a secret backend

To update a secret backend on the controller, run the `update-secret-backend` command followed by the name of the secret backend. Below we update the backend by supplying a configuration from a file:

```text
juju update-secret-backend myvault --config /path/to/cfg.yaml
```

> See more: {ref}`command-juju-update-secret-backend`

## Remove a secret backend

To remove a secret backend, use the `remove-secret-backend` command followed by the backend name:

```text
juju remove-secret-backend myvault
```

> See more: {ref}`command-juju-update-secret-backend`

