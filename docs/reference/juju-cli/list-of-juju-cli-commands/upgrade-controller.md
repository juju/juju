(command-juju-upgrade-controller)=
# `juju upgrade-controller`
> See also: [upgrade-model](#upgrade-model)

## Summary
Upgrades Juju on a controller.

## Usage
```juju upgrade-controller [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--agent-stream` |  | Specifies the agent stream to check for upgrades. |
| `--agent-version` |  | Upgrades to specific version. |
| `--build-agent` | false | (JUJU DEVELOPERS ONLY) Specifies whether to build a local version of the agent binary. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--dry-run` | false | Specifies whether to merely simulate the upgrade. |
| `--ignore-agent-versions` | false | Does not check if all agents have already reached the current version. |
| `--reset-previous-upgrade` | false | (USE WITH CARE) Specifies whether to clear the previous (incomplete) upgrade status. |
| `--timeout` | 10m0s | Specifies the timeout before upgrade is aborted. |
| `-y`, `--yes` | false | Specifies whether to answer 'yes' to confirmation prompts. |

## Examples

    juju upgrade-controller --dry-run
    juju upgrade-controller --agent-version 2.0.1


## Details
This command upgrades the Juju agent for a controller.

A controller's agent version can be shown with `juju model-config -m controller agent-version`.
A version is denoted by: `major.minor.patch`.

You can upgrade the controller to a new patch version by specifying
the `--agent-version` flag. If not specified, the upgrade candidate
will default to the most recent patch version matching the current
major and minor version. Upgrading to a new major or minor version is
not supported.

The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g., if one of the
controllers in a high availability model failed to upgrade).