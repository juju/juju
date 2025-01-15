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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--format` | yaml | Specify output format (yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--show-secrets` | false | Display credential secret attributes |

## Examples

    juju show-credential google my-admin-credential
    juju show-credentials 
    juju show-credentials --controller mycontroller --client 
    juju show-credentials --controller mycontroller 
    juju show-credentials --client
    juju show-credentials --show-secrets


## Details

This command displays information about cloud credential(s) stored 
either on this client or on a controller for this user.

To see the contents of a specific credential, supply its cloud and name.
To see all credentials stored for you, supply no arguments.

To see secrets, content attributes marked as hidden, use --show-secrets option.

To see credentials from this client, use "--client" option.

To see credentials from a controller, use "--controller" option.