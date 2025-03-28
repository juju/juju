(command-juju-attach-resource)=
# `juju attach-resource`

```
Usage: juju attach-resource [options] application name=file|OCI image

Summary:
Update a resource for an application.

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

Details:
This command updates a resource for an application.

For file resources, it uploads a file from your local disk to the juju controller to be
streamed to the charm when "resource-get" is called by a hook.

For OCI image resources used by k8s applications, an OCI image or file path is specified.
A file is specified when a private OCI image is needed and the username/password used to
access the image is needed along with the image path.

Aliases: attach
```