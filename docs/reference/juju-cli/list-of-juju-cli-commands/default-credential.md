(command-juju-default-credential)=
# `juju default-credential`

```
Usage: juju default-credential [options] <cloud name> [<credential name>]

Summary:
Sets local default credentials for a cloud on this client.

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
    Reset default credential for the cloud

Details:
The default credentials are specified with a "credential name".

A credential name is created during the process of adding credentials either
via `juju add-credential` or `juju autoload-credentials`.
Credential names can be listed with `juju credentials`.

This command sets a locally stored credential to be used as a default.
Default credentials avoid the need to specify a particular set of
credentials when more than one are available for a given cloud.

To unset previously set default credential for a cloud, use --reset option.

To view currently set default credential for a cloud, use the command
without a credential name argument.

Examples:
    juju default-credential google credential_name
    juju default-credential google
    juju default-credential google --reset

See also:
    credentials
    add-credential
    remove-credential
    autoload-credentials

Aliases: set-default-credential
```