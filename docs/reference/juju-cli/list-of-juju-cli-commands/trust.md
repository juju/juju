(command-juju-trust)=
# `juju trust`

```
Usage: juju trust [options] <application name>

Summary:
Sets the trust status of a deployed application to true.

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
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--remove  (= false)
    Remove trusted access from a trusted application
--scope (= "")
    k8s models only - needs to be set to 'cluster'

Details:
Sets the trust configuration value to true.

On k8s models, the trust operation currently grants the charm full access to the cluster.
Until the permissions model is refined to grant more granular role based access, the use of
'--scope=cluster' is required to confirm this choice.

Examples:
    juju trust media-wiki
    juju trust metallb --scope=cluster

See also:
    config
```