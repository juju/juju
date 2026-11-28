(command-juju-update-secret-backend)=
# `juju update-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [remove-secret-backend](#command-juju-remove-secret-backend), [show-secret-backend](#command-juju-show-secret-backend)

## Summary
Update an existing secret backend on the controller.

## Usage
```text
juju update-secret-backend [options] <backend-name> (--config <file> | <key>=<value> | --reset <key>) ...
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--config` |  | Path to yaml-formatted configuration file |
| `--force` | false | Force update even if the backend is unreachable |
| `--reset` |  | Reset one provider configuration key; repeat for each key |

## Examples

Apply the settings in a private YAML file to the existing Vault backend `myvault`:

    juju update-secret-backend myvault --config vault_config.yaml

Enable token refresh and request a two-hour lifetime for Juju's Vault access token:

    juju update-secret-backend myvault token-rotate=2h

Disable automatic refresh of the Vault access token:

    juju update-secret-backend myvault token-rotate=0

Clear the configured TLS client certificate and its private key:

    juju update-secret-backend myvault --reset client-cert --reset client-key

Rename the registration from `myvault` to `team-vault`:

    juju update-secret-backend myvault name=team-vault


## Details

Updates a backend by its current name. Supply a YAML mapping with `--config`
(use - for stdin), key=value arguments, or `--reset`. Arguments override file
entries. Repeat `--reset` once per provider key; commas do not separate keys.
Resets are applied last. To disable backend token refresh, use token-rotate=0,
not `--reset token-rotate`. Use name=NEW to rename the registration.

On success the command prints nothing; use show-secret-backend with the
resulting name to inspect the change.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/