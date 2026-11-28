(command-juju-add-secret-backend)=
# `juju add-secret-backend`
> See also: [secret-backends](#command-juju-secret-backends), [remove-secret-backend](#command-juju-remove-secret-backend), [show-secret-backend](#command-juju-show-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Add a new secret backend to the controller.

## Usage
```text
juju add-secret-backend [options] <backend-name> <backend-type> (--config <file> | <key>=<value> ...) [<key>=<value> ...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | YAML configuration file, or - for stdin |
| `--import-id` |  | Add the backend with the specified ID |

## Examples

    juju add-secret-backend myvault vault --config vault_config.yaml
    juju add-secret-backend myvault vault --config vault_config.yaml token-rotate=2h


## Details

Registers a store on the controller; it does not select a backend for a model.
Supply configuration as key=value arguments, a YAML mapping with `--config`, or
both. Use `--config -` to read stdin. Arguments override keys from the file.
For a provider that supports token refresh, token-rotate takes a duration
of at least one hour. Omit it to register without requesting refresh.

The command prints no ID on success. Omit `--import-id` for a new registration;
supplying an existing ID can update that registration instead.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/