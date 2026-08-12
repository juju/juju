(command-juju-remove-secret-backend)=
# `juju remove-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [show-secret-backend](#command-juju-show-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Removes a secret backend from the controller.

## Usage
```text
juju remove-secret-backend [options] <backend-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--force` | false | Force removal even if the backend stores in-use secrets |

## Examples

    juju remove-secret-backend myvault
    juju remove-secret-backend myvault --force


## Details

Removes a secret backend, used for storing secret content.
If the backend is being used to store secrets currently in use,
the `--force` option can be supplied to force the removal, but be
warned, this will affect charms which use those secrets.