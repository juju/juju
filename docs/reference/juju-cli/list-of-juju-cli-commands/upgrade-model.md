(command-juju-upgrade-model)=
# `juju upgrade-model`
> See also: [sync-agent-binary](#sync-agent-binary)

## Summary
Upgrades Juju on all machines in a model.

## Usage
```juju upgrade-model [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--agent-stream` |  | Specifies the agent stream to check for upgrades. |
| `--agent-version` |  | Upgrades to specific version. |
| `--dry-run` | false | Simulates the upgrade. |
| `--ignore-agent-versions` | false | Does not check if all agents have already reached the current version. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--reset-previous-upgrade` | false | Clears the previous (incomplete) upgrade status (use with care). |
| `--timeout` | 10m0s | Specifies the timeout before upgrade is aborted. |
| `-y`, `--yes` | false | Answers 'yes' to confirmation prompts. |

## Examples

    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed


## Details
Juju provides agent software to every machine it creates. This command
upgrades that software across an entire model, which is, by default, the
current model.

A model's agent version can be shown with `juju model-config agent-version`.
A version is denoted by: `major.minor.patch`

If `--agent-version` is not specified, then the upgrade candidate is
selected to be the exact version the controller itself is running.

If the controller is without internet access, the client must first supply
the software to the controller's cache via the `juju sync-agent-binary` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g., if one of the
controllers in a high availability model failed to upgrade).

When looking for an agent to upgrade to, Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using `--agent-stream`.

If a failed upgrade has been resolved, `--reset-previous-upgrade` can be
used to allow the upgrade to proceed.
Backups are recommended prior to upgrading.