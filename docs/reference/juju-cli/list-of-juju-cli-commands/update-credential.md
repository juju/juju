(command-juju-update-credential)=
# `juju update-credential`
> See also: [add-credential](#add-credential), [credentials](#credentials), [remove-credential](#remove-credential), [set-credential](#set-credential)

**Aliases:** update-credentials

## Summary
Updates a controller credential for a cloud.

## Usage
```juju update-credential [options] [<cloud-name> [<credential-name>]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |
| `-f`, `--file` |  | Specifies the YAML file containing credential details to update. |
| `--force` | false | Forcibly updates controller side credential, ignoring validation errors. |
| `--region` |  | Specifies the cloud region that the credential is valid for. |

## Examples

    juju update-credential aws mysecrets
    juju update-credential -f mine.yaml
    juju update-credential -f mine.yaml --client
    juju update-credential aws -f mine.yaml
    juju update-credential azure --region brazilsouth -f mine.yaml
    juju update-credential -f mine.yaml --controller mycontroller --force


## Details

Cloud credentials are used for model operations and manipulations.
Since it is common to have long-running models, it is also common to
have these cloud credentials become invalid during a model's lifetime.
When this happens, the cloud credential that a model was created with
must be updated to the new and valid details on the controller.

This command allows updating an existing, already-stored, named,
cloud-specific credential on a controller as well as the one from this client.

The `--controller `option can be used to update a credential definition on a controller.

When updating cloud credential on a controller, Juju performs additional
checks to ensure that the models that use this credential can still
access cloud instances after the update. Occasionally, these checks may not be desired
by the user and can be by-passed using the `--force` option.
Force update may leave some models with unreachable machines.
Consequently, it is not recommended as a default update action.
Models with unreachable machines are most commonly fixed by using another cloud credential,
see `juju set-credential` for more information.

The `--client` option can be used to update a credential definition on this client.
If a different client will be used, say a different laptop,
the update will not affect that client's (laptop's) copy.

Before credential is updated, the new content is validated. For some providers,
cloud credentials are region specific. To validate the credential for a non-default region,
use `--region`