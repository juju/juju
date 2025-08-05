> See also: [add-credential](#add-credential), [credentials](#credentials), [remove-credential](#remove-credential), [set-credential](#set-credential)

**Aliases:** update-credentials

## Summary
Updates a controller credential for a cloud.

## Usage
```juju update-credential [options] [<cloud-name> [<credential-name>]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `-f`, `--file` |  | The YAML file containing credential details to update |
| `--force` | false | Force update controller side credential, ignore validation errors |
| `--region` |  | Cloud region that credential is valid for |

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
have these cloud credentials become invalid during models' lifetime.
When this happens, a user must update the cloud credential that 
a model was created with to the new and valid details on controller.

This command allows to update an existing, already-stored, named,
cloud-specific credential on a controller as well as the one from this client.

Use --controller option to update a credential definition on a controller. 

When updating cloud credential on a controller, Juju performs additional
checks to ensure that the models that use this credential can still
access cloud instances after the update. Occasionally, these checks may not be desired
by the user and can be by-passed using --force option. 
Force update may leave some models with un-reachable machines.
Consequently, it is not recommended as a default update action.
Models with un-reachable machines are most commonly fixed by using another cloud credential, 
see ' + "'juju set-credential'" + ' for more information.

Use --client to update a credential definition on this client.
If a user will use a different client, say a different laptop, 
the update will not affect that client's (laptop's) copy.

Before credential is updated, the new content is validated. For some providers, 
cloud credentials are region specific. To validate the credential for a non-default region, 
use --region.




