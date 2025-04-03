(command-juju-model-defaults)=
# `juju model-defaults`

```
Usage: juju model-defaults [options] [[<cloud>/]<region> ]<model-key>[<=value>] ...]

Summary:
Displays or sets default configuration settings for new models.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--ignore-read-only-fields  (= false)
    Ignore read only fields that might cause errors to be emitted while processing yaml documents
-o, --output (= "")
    Specify an output file
--reset  (= )
    Reset the provided comma delimited keys

Details:
By default, all default configuration (keys and values) are
displayed if a key is not specified. Supplying key=value will set the
supplied key to the supplied value. This can be repeated for multiple keys.
You can also specify a yaml file containing key values.

Model default configuration settings are specific to the cloud on which the
model is deployed.

If the controller host more then one cloud, the cloud (and optionally region)
must be specified.

Model defaults yaml configuration can be piped from stdin from the output in
yaml format of the command stdout.

Some model-defaults configuration are read-only, to prevent
the command exiting on read-only fields, setting "ignore-read-only-fields" will
cause it to skip over the fields when they're encountered.

Examples:

Display all model config default values
    juju model-defaults

Display the value of http-proxy model config default
    juju model-defaults http-proxy

Display the value of http-proxy model config default for the aws cloud
    juju model-defaults aws http-proxy

Display the value of http-proxy model config default for the aws cloud
and us-east-1 region
    juju model-defaults aws/us-east-1 http-proxy

Display the value of http-proxy model config default for the us-east-1 region
    juju model-defaults us-east-1 http-proxy

Set the value of ftp-proxy model config default to 10.0.0.1:8000
    juju model-defaults ftp-proxy=10.0.0.1:8000

Set model default values for the us-east-1 region as defined in
path/to/file.yaml and ftp-proxy on the command line
    juju model-defaults us-east-1 ftp-proxy=10.0.0.1:8000 path/to/file.yaml

Set model default values for the aws cloud as defined in path/to/file.yaml
    juju model-defaults aws path/to/file.yaml

Reset the value of default-series and test-mode to default
    juju model-defaults --reset default-series,test-mode

Reset the value of http-proxy for the us-east-1 region to default
    juju model-defaults us-east-1 --reset http-proxy

See also:
    models
    model-config

Aliases: model-default
```