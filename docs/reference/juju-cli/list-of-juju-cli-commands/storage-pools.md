(command-juju-storage-pools)=
# `juju storage-pools`

```
Usage: juju storage-pools [options]

Summary:
List storage pools.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--name  (= )
    Only show pools with these names
-o, --output (= "")
    Specify an output file
--provider  (= )
    Only show pools of these provider types

Details:
The user can filter on pool type, name.

If no filter is specified, all current pools are listed.
If at least 1 name and type is specified, only pools that match both a name
AND a type from criteria are listed.
If only names are specified, only mentioned pools will be listed.
If only types are specified, all pools of the specified types will be listed.

Both pool types and names must be valid.
Valid pool types are pool types that are registered for Juju model.

Aliases: list-storage-pools
```