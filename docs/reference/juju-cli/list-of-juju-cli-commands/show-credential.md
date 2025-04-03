(command-juju-show-credential)=
# `juju show-credential`

```
Usage: juju show-credential [options] [<cloud name> <credential name>]

Summary:
Shows credential information stored either on this client or on a controller.

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
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
-o, --output (= "")
    Specify an output file
--show-secrets  (= false)
    Display credential secret attributes

Details:
This command displays information about cloud credential(s) stored
either on this client or on a controller for this user.

To see the contents of a specific credential, supply its cloud and name.
To see all credentials stored for you, supply no arguments.

To see secrets, content attributes marked as hidden, use --show-secrets option.

To see credentials from this client, use "--client" option.

To see credentials from a controller, use "--controller" option.

Examples:

    juju show-credential google my-admin-credential
    juju show-credentials
    juju show-credentials --controller mycontroller --client
    juju show-credentials --controller mycontroller
    juju show-credentials --client
    juju show-credentials --show-secrets

See also:
    credentials
    add-credential
    update-credential
    remove-credential
    autoload-credentials

Aliases: show-credentials
```