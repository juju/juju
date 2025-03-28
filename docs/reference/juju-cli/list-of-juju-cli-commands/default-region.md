(command-juju-default-region)=
# `juju default-region`

```
Usage: juju default-region [options] <cloud name> [<region>]

Summary:
Sets the default region for a cloud.

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
--reset  (= false)
    Reset default region for the cloud

Details:
The default region is specified directly as an argument.

To unset previously set default region for a cloud, use --reset option.

To confirm what region is currently set to be default for a cloud,
use the command without region argument.

Examples:
    juju default-region azure-china chinaeast
    juju default-region azure-china
    juju default-region azure-china --reset

See also:
    add-credential

Aliases: set-default-region
```