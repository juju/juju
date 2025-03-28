(command-juju-show-cloud)=
# `juju show-cloud`

```
Usage: juju show-cloud [options] <cloud name>

Summary:
Shows detailed information for a cloud.

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
--format  (= yaml)
    Specify output format (yaml)
--include-config  (= false)
    Print available config option details specific to the specified cloud
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
-o, --output (= "")
    Specify an output file

Details:
Provided information includes 'defined' (public, built-in), 'type',
'auth-type', 'regions', 'endpoints', and cloud specific configuration
options.

If ‘--include-config’ is used, additional configuration (key, type, and
description) specific to the cloud are displayed if available.

Use --controller option to show a cloud from a controller.

Use --client option to show a cloud known on this client.

Examples:

    juju show-cloud google
    juju show-cloud azure-china --output ~/azure_cloud_details.txt
    juju show-cloud myopenstack --controller mycontroller
    juju show-cloud myopenstack --client
    juju show-cloud myopenstack --client --controller mycontroller

See also:
    clouds
    add-cloud
    update-cloud
```