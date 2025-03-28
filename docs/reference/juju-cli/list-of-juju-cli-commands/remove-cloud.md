(command-juju-remove-cloud)=
# `juju remove-cloud`

```
Usage: juju remove-cloud [options] <cloud name>

Summary:
Removes a cloud from Juju.

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
-c, --controller (= "")
    Controller to operate in
--client  (= false)
    Client operation
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected

Details:
Remove a cloud from Juju.

If --controller is used, also remove the cloud from the specified controller,
if it is not in use.

If --client is specified, Juju removes the cloud from this client.

Examples:
    juju remove-cloud mycloud
    juju remove-cloud mycloud --client
    juju remove-cloud mycloud --controller mycontroller

See also:
    add-cloud
    update-cloud
    list-clouds
```