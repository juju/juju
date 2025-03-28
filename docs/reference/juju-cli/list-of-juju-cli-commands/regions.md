(command-juju-regions)=
# `juju regions`

```
Usage: juju regions [options] <cloud>

Summary:
Lists regions for a given cloud.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
-o, --output (= "")
    Specify an output file

Details:
List regions for a given cloud.

Use --controller option to list regions from the cloud from a controller.

Use --client option to list regions known locally on this client.


Examples:

    juju regions aws
    juju regions aws --controller mycontroller
    juju regions aws --client
    juju regions aws --client --controller mycontroller

See also:
    add-cloud
    clouds
    show-cloud
    update-cloud
    update-public-clouds

Aliases: list-regions
```