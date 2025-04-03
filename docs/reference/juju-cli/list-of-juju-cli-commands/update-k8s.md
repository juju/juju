(command-juju-update-k8s)=
# `juju update-k8s`

```
Usage: juju update-k8s [options] <k8s name>

Summary:
Updates an existing k8s endpoint used by Juju.

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
Update k8s cloud information on this client and/or on a controller.

The k8s cloud can be a built-in cloud like microk8s.

A k8s cloud can also be updated from a file. This requires a <cloud name> and
a yaml file containing the cloud details.

A k8s cloud on the controller can also be updated just by using a name of a k8s cloud
from this client.

Use --controller option to update a k8s cloud on a controller.

Use --client to update a k8s cloud definition on this client.

Examples:
    juju update-k8s microk8s
    juju update-k8s myk8s -f path/to/k8s.yaml
    juju update-k8s myk8s -f path/to/k8s.yaml --controller mycontroller
    juju update-k8s myk8s --controller mycontroller
    juju update-k8s myk8s --client --controller mycontroller
    juju update-k8s myk8s --client -f path/to/k8s.yaml

See also:
    add-k8s
    remove-k8s
```