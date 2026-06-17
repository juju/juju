(command-juju-update-secret-backend)=
# `juju update-secret-backend`
> See also: [add-secret-backend](#add-secret-backend), [secret-backends](#secret-backends), [remove-secret-backend](#remove-secret-backend), [show-secret-backend](#show-secret-backend)

## Summary
Update an existing secret backend on the controller.

## Usage
```juju update-secret-backend [options] <backend-name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | Path to yaml-formatted configuration file |
| `--force` | false | Force update even if the backend is unreachable |
| `--reset` |  | Reset the provided comma delimited config keys |

## Examples

    juju update-secret-backend myvault --config /path/to/cfg.yaml
    juju update-secret-backend myvault name=myvault2
    juju update-secret-backend myvault token-rotate=10m --config /path/to/cfg.yaml
    juju update-secret-backend myvault endpoint=https://vault.io:8200 token=s.1wshwhw
    juju update-secret-backend myvault token-rotate=0
    juju update-secret-backend myvault --reset namespace,ca-cert


## Details

Updates a new secret backend for storing secret content.

You must specify a name for the backend to update,
followed by any necessary backend specific config values.
Config may be specified as key values ot read from a file.
Any key values override file content if both are specified.

Config attributes may be reset back to the default value using `--reset`.

To rotate the backend access credential/token (if specified), use
the `token-rotate` config and supply a duration. To reset any existing
token rotation period, supply a value of `0`.