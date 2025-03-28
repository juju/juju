(command-juju-remove-k8s)=
# `juju remove-k8s`

```
Usage: juju remove-k8s [options] <k8s name>

Summary:
Removes a k8s cloud from Juju.

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
Removes the specified k8s cloud from this client.

If --controller is used, also removes the cloud
from the specified controller (if it is not in use).

Use --client option to update your current client.

Examples:
    juju remove-k8s myk8scloud
    juju remove-k8s myk8scloud --client
    juju remove-k8s --controller mycontroller myk8scloud

See also:
    add-k8s
```