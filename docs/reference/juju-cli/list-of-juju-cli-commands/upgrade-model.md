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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--agent-stream` |  | Check this agent stream for upgrades |
| `--agent-version` |  | Upgrade to specific version |
| `--dry-run` | false | Don't change anything, just report what would be changed |
| `--ignore-agent-versions` | false | Don't check if all agents have already reached the current version |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--reset-previous-upgrade` | false | Clear the previous (incomplete) upgrade status (use with care) |
| `--timeout` | 10m0s | Timeout before upgrade is aborted |
| `-y`, `--yes` | false | Answer 'yes' to confirmation prompts |

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