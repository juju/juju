(command-juju-add-model)=
# `juju add-model`
> See also: [model-config](#command-juju-model-config), [model-defaults](#command-juju-model-defaults), [add-credential](#command-juju-add-credential), [autoload-credentials](#command-juju-autoload-credentials)

## Summary
Adds a workload model.

## Usage
```text
juju add-model [options] <model name> [cloud|region|(cloud/region)]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | Specify the path to a YAML model configuration file or individual configuration options (`--config config.yaml [--config key=value ...]`) |
| `--credential` |  | Specify the credential to be used by the model |
| `--no-switch` | false | Choose not to switch to the newly created model |
| `--owner` |  | Select the user qualifier for the model name (controller superuser for another user) |
| `--target-controller` |  | Select a managed controller when using the JAAS plugin |

## Examples

Create and switch to a model when one cloud is available:

    juju add-model mymodel

Select a region when AWS is the only available cloud:

    juju add-model mymodel us-east-1

Select a cloud and region:

    juju add-model mymodel aws/us-east-1

Configure a model on AWS from a file and override one setting:

    juju add-model mymodel aws --config my-config.yaml --config image-stream=daily

Choose a named credential:

    juju add-model mymodel aws --credential production


## Details
Adding a model is typically done in order to run a specific workload.

To add a model, you must specify a model name. Model names can be duplicated
across controllers but must be unique per user for any given controller.
In other words, Alice and Bob can each have their own model called "secret" but
Alice can have only one model called "secret" in a controller.
Model names may only contain lowercase letters, digits and hyphens, and
may not start with a hyphen.

A controller superuser may select another user's model-name qualifier with
`--owner`. The signed-in creator receives model admin access. If you
select another user's qualifier, the CLI leaves your current model unchanged.

With `--credential NAME`, the CLI first looks for that name on this
client. It uploads a matching local copy under the selected user's name,
even if that replaces a controller copy. If there is no local match, it
looks for the named credential on the controller.

Without a credential name, if the cloud supports empty authentication,
the CLI sends no credential identifier. The controller selects the signed-in
user's sole valid, non-revoked credential for that cloud. If there is no
unique match, it creates the model without a credential.

On a cloud that requires authentication, the CLI looks for a single
credential for the selected user and cloud on the controller, then tries
local selection. A locally selected name must already exist on the controller.
To upload a local copy as part of model creation, name it explicitly.

Supply model settings with `--config` and select a cloud and region
with `cloud/region`. If exactly one cloud is available to you, you
may omit the cloud or specify a region in that cloud.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/model/