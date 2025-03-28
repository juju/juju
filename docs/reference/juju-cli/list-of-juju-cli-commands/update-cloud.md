(command-juju-update-cloud)=
# `juju update-cloud`

```
Usage: juju update-cloud [options] <cloud name>

Summary:
Updates cloud information available to Juju.

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
-f (= "")
    The path to a cloud definition file
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected

Details:
Update cloud information on this client and/or on a controller.

A cloud can be updated from a file. This requires a <cloud name> and a yaml file
containing the cloud details.
This method can be used for cloud updates on the client side and on a controller.

A cloud on the controller can also be updated just by using a name of a cloud
from this client.

Use --controller option to update a cloud on a controller.

Use --client to update cloud definition on this client.

Examples:

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller
    juju update-cloud mymaas --client --controller mycontroller
    juju update-cloud mymaas --client -f path/to/maas.yaml

See also:
    add-cloud
    remove-cloud
    list-clouds
```