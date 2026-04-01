(command-juju-add-model)=
# `juju add-model`
> See also: [model-config](#model-config), [model-defaults](#model-defaults), [add-credential](#add-credential), [autoload-credentials](#autoload-credentials)

## Summary
Adds a workload model.

## Usage
```juju add-model [options] <model name> [cloud|region|(cloud/region)]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | Specify the path to a YAML model configuration file or individual configuration options (`--config config.yaml [--config key=value ...]`) |
| `--credential` |  | Specify the credential to be used by the model |
| `--no-switch` | false | Choose not to switch to the newly created model |
| `--owner` |  | Specify the user who will own the model, if not the current user |
| `--target-controller` |  | The name of a JAAS managed controller to add a model to |

## Examples

    juju add-model mymodel
    juju add-model mymodel us-east-1
    juju add-model mymodel aws/us-east-1
    juju add-model mymodel --config my-config.yaml --config image-stream=daily
    juju add-model mymodel --credential credential_name --config authorized-keys="ssh-rsa ..."


## Details
Adding a model is typically done in order to run a specific workload.

To add a model, you must specify a model name. Model names can be duplicated
across controllers but must be unique per user for any given controller.
In other words, Alice and Bob can each have their own model called "secret" but
Alice can have only one model called "secret" in a controller.
Model names may only contain lowercase letters, digits and hyphens, and
may not start with a hyphen.

To add a model, Juju requires a credential:
* if you have a default (or just one) credential defined on your client
  (i.e. in `credentials.yaml`), then juju will use that;
* if you have no default (or multiple) credentials defined on your client,
  then you must specify one using `--credential`;
* as the admin user you can omit the credential,
  and the credential used to bootstrap will be used.

To add a credential for add-model, use one of the `juju add-credential` or
`juju autoload-credentials` commands. These will add credentials
to the Juju client, which `juju add-model` will upload to the controller
as necessary.

A model's credential (whether specified explicitly or used by default) will be
visible to users who have be granted write or admin access to the model.
This is because trusted charms may request the credential to create resources on
the cloud on behalf of the model.

Juju uses the BYO credential paradigm, meaning that users who have been granted
add-model permission on a cloud are able to create their own model(s) on that
cloud, but they are also then expected to supply their own credential.
As a controller superuser, it is therefore not recommended that you create
models for other users using your own credential, or any credential that you do
not want to be potentially visible to charms or other users with model write
access.

You may also supply model-specific configuration as well as a
cloud/region to which this model will be deployed. The cloud/region and credentials
are the ones used to create any future resources within the model.

If no cloud/region is specified, then the model will be deployed to
the same cloud/region as the controller model. If a region is specified
without a cloud qualifier, then it is assumed to be in the same cloud
as the controller model.

When adding `--config`, the `default-series` key is deprecated in favour of
`default-base`, e.g. `ubuntu@22.04`.