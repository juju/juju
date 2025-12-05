(command-juju-show-credential)=
# `juju show-credential`
> See also: [credentials](#credentials), [add-credential](#add-credential), [update-credential](#update-credential), [remove-credential](#remove-credential), [autoload-credentials](#autoload-credentials)

**Aliases:** show-credentials

## Summary
Shows credential information stored either on this client or on a controller.

## Usage
```juju show-credential [options] [<cloud name> <credential name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |
| `--format` | yaml | Specify output format (yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--show-secrets` | false | Displays credential secret attributes. |

## Examples

    juju show-credential google my-admin-credential
    juju show-credentials
    juju show-credentials --controller mycontroller --client
    juju show-credentials --controller mycontroller
    juju show-credentials --client
    juju show-credentials --show-secrets


## Details

Displays information about cloud credential(s) stored
either on this client or on a controller for this user.

The cloud and name can be supplied to see the contents of a specific credential.
If no arguments are supplied, all credentials stored for the user are displayed.

The `--show-secrets` option can be used to see secrets, content attributes marked as hidden.

The `--client` option can be used to see credentials from this client.

The `--controller` option can be used to see credentials from a controller.