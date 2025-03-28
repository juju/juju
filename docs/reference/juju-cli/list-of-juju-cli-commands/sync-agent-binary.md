(command-juju-sync-agent-binary)=
# `juju sync-agent-binary`

```
Usage: juju sync-agent-binary [options]

Summary:
Copy agent binaries from the official agent store into a local controller.

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
--agent-version (= "")
    Copy a specific major[.minor] version
--dry-run  (= false)
    Don't copy, just print what would be copied
--local-dir (= "")
    Local destination directory
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--public  (= false)
    Tools are for a public cloud, so generate mirrors information
--source (= "")
    Local source directory
--stream (= "")
    Simplestreams stream for which to sync metadata

Details:
This copies the Juju agent software from the official agent binaries store
(located at https://streams.canonical.com/juju) into the controller.
It is generally done when the controller is without Internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.

Examples:
    juju sync-agent-binary --debug --agent-version 2.0
    juju sync-agent-binary --debug --agent-version 2.0 --local-dir=/home/ubuntu/sync-agent-binary

See also:
    upgrade-controller

Aliases: sync-tools
```