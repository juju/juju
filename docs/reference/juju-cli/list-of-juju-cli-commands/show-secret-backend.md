(command-juju-show-secret-backend)=
# `juju show-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [remove-secret-backend](#command-juju-remove-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Displays the specified secret backend.

## Usage
```text
juju show-secret-backend [options] <backend-name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-c`, `--controller` |  | Controller to operate in |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--reveal` | false | Include sensitive backend config content |

## Examples

Inspect the `myvault` registration without displaying sensitive settings:

    juju show-secret-backend myvault

Include sensitive backend settings in the output, such as the Vault access token:

    juju show-secret-backend myvault --reveal


## Details

Displays the registration matching the exact name, as YAML by default or JSON.
Ordinary UUIDs are not printed. Use `--reveal` only if you need sensitive backend
settings, not secret values. If no name matches, the command reports that no
backends have been added and succeeds without writing formatted output.
Other backends can still exist on the controller.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/