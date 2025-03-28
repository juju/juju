(command-juju-upgrade-controller)=
# `juju upgrade-controller`

```
Usage: juju upgrade-controller [options]

Summary:
Upgrades Juju on a controller.

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
-c, --controller (= "")
    Controller to operate in
--dry-run  (= false)
    Don't change anything, just report what would be changed
--ignore-agent-versions  (= false)
    Don't check if all agents have already reached the current version
--reset-previous-upgrade  (= false)
    Clear the previous (incomplete) upgrade status (use with care)
--timeout  (= 10m0s)
    Timeout before upgrade is aborted
-y, --yes  (= false)
    Answer 'yes' to confirmation prompts

Details:
This command upgrades the Juju agent for a controller.

A controller's agent version can be shown with `juju model-config -m controller agent-
version`.
A version is denoted by: major.minor.patch
The upgrade candidate will be auto-selected if '--agent-version' is not
specified:
 - If the server major version matches the client major version, the
 version selected is minor+1. If such a minor version is not available then
 the next patch version is chosen.
 - If the server major version does not match the client major version,
 the version selected is that of the client version.
The command will abort if an upgrade is in progress. It will also abort if
a previous upgrade was not fully completed (e.g.: if one of the
controllers in a high availability model failed to upgrade).

Examples:
    juju upgrade-controller --dry-run
    juju upgrade-controller --agent-version 2.0.1

See also:
    upgrade-model
```