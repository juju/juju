(command-juju-remove-credential)=
# `juju remove-credential`
> See also: [add-credential](#add-credential), [autoload-credentials](#autoload-credentials), [credentials](#credentials), [default-credential](#default-credential), [set-credential](#set-credential), [update-credential](#update-credential)

## Summary
Removes Juju credentials for a cloud.

## Usage
```juju remove-credential [options] <cloud name> <credential name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |
| `--force` | false | Forcibly removes a controller-side credential, ignoring validation errors. |

## Examples

    juju remove-credential google credential_name
    juju remove-credential google credential_name --client
    juju remove-credential google credential_name -c mycontroller
    juju remove-credential google credential_name -c mycontroller --force



## Details
The credential to be removed is specified by a credential name.
Credential names, and optionally the corresponding authentication
material, can be listed with `juju credentials`.

Use the `--controller` option to remove credentials from a controller.

When removing cloud credential from a controller, Juju performs additional
checks to ensure that there are no models using this credential.
Occasionally, these check may not be desired by the user and can be by-passed using `--force`.
If force remove was performed and some models were still using the credential, these models
will be left with unreachable machines.
Consequently, it is not recommended as a default remove action.
Models with unreachable machines are most commonly fixed by using another cloud credential,
see `juju set-credential` for more information.


Use the `--client` option to remove credentials from the current client.