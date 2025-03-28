(command-juju-upgrade-model)=
# `juju upgrade-model`

```
Usage: juju upgrade-model [options]

Summary:
Upgrades Juju on all machines in a model.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
--agent-stream (= "")
    Check this agent stream for upgrades
--agent-version (= "")
    Upgrade to specific version
--build-agent  (= false)
    Build a local version of the agent binary; for development use only
--dry-run  (= false)
    Don't change anything, just report what would be changed
--ignore-agent-versions  (= false)
    Don't check if all agents have already reached the current version
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--reset-previous-upgrade  (= false)
    Clear the previous (incomplete) upgrade status (use with care)
--timeout  (= 10m0s)
    Timeout before upgrade is aborted
-y, --yes  (= false)
    Answer 'yes' to confirmation prompts

Details:
Juju provides agent software to every machine it creates. This command
upgrades that software across an entire model, which is, by default, the
current model.
A model's agent version can be shown with `juju model-config agent-
version`.
A version is denoted by: major.minor.patch
The upgrade candidate will be auto-selected if '--agent-version' is not
specified:
 - If the server major version matches the client major version, the
 version selected is minor+1. If such a minor version is not available then
 the next patch version is chosen.
 - If the server major version does not match the client major version,
 the version selected is that of the client version.
If the controller is without internet access, the client must first supply
the software to the controller's cache via the `juju sync-agent-binary` command.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).
When looking for an agent to upgrade to Juju will check the currently
configured agent stream for that model. It's possible to overwrite this for
the lifetime of this upgrade using --agent-stream
If a failed upgrade has been resolved, '--reset-previous-upgrade' can be
used to allow the upgrade to proceed.
Backups are recommended prior to upgrading.

Examples:
    juju upgrade-model --dry-run
    juju upgrade-model --agent-version 2.0.1
    juju upgrade-model --agent-stream proposed

See also:
    sync-agent-binary

Aliases: upgrade-juju
```