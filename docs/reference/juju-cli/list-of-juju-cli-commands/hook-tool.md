(command-juju-hook-tool)=
# `juju hook-tool`

```
Usage: juju hook-tool [tool]

Summary:
Show help on a Juju charm hook tool.

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

Details:
Juju charms can access a series of built-in helpers called 'hook-tools'.
These are useful for the charm to be able to inspect its running environment.
Currently available charm hook tools are:

    action-fail              set action fail status with message
    action-get               get action parameters
    action-log               record a progress message for the current action
    action-set               set action results
    add-metric               add metrics
    application-version-set  specify which version of the application is deployed
    close-port               register a request to close a port or port range
    config-get               print application configuration
    credential-get           access cloud credentials
    goal-state               print the status of the charm's peers and related units
    is-leader                print application leadership status
    juju-log                 write a message to the juju log
    juju-reboot              Reboot the host machine
    k8s-raw-get              get k8s raw spec information
    k8s-raw-set              set k8s raw spec information
    k8s-spec-get             get k8s spec information
    k8s-spec-set             set k8s spec information
    leader-get               print application leadership settings
    leader-set               write application leadership settings
    network-get              get network config
    open-port                register a request to open a port or port range
    opened-ports             list all ports or port ranges opened by the unit
    payload-register         register a charm payload with juju
    payload-status-set       update the status of a payload
    payload-unregister       stop tracking a payload
    pod-spec-get             get k8s spec information (deprecated)
    pod-spec-set             set k8s spec information (deprecated)
    relation-get             get relation settings
    relation-ids             list all relation ids with the given relation name
    relation-list            list relation units
    relation-set             set relation settings
    resource-get             get the path to the locally cached resource file
    state-delete             delete server-side-state key value pair
    state-get                print server-side-state value
    state-set                set server-side-state values
    status-get               print status information
    status-set               set status information
    storage-add              add storage instances
    storage-get              print information for storage instance with specified id
    storage-list             list storage attached to the unit
    unit-get                 print public-address or private-address

Examples:

    For help on a specific tool, supply the name of that tool, for example:

        juju hook-tool unit-get

Aliases: help-tool, hook-tools
```