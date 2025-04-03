(command-juju-remove-credential)=
# `juju remove-credential`

```
Usage: juju remove-credential [options] <cloud name> <credential name>

Summary:
Removes Juju credentials for a cloud.

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
--force  (= false)
    Force remove controller side credential, ignore validation errors
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected

Details:
The credential to be removed is specified by a "credential name".
Credential names, and optionally the corresponding authentication
material, can be listed with `juju credentials`.

Use --controller option to remove credentials from a controller.

When removing cloud credential from a controller, Juju performs additional
checks to ensure that there are no models using this credential.
Occasionally, these check may not be desired by the user and can be by-passed using --force.
If force remove was performed and some models were still using the credential, these models
will be left with un-reachable machines.
Consequently, it is not recommended as a default remove action.
Models with un-reachable machines are most commonly fixed by using another cloud credential,
see ' + "'juju set-credential'" + ' for more information.


Use --client option to remove credentials from the current client.

Examples:
    juju remove-credential google credential_name
    juju remove-credential google credential_name --client
    juju remove-credential google credential_name -c mycontroller
    juju remove-credential google credential_name -c mycontroller --force

See also:
    add-credential
    autoload-credentials
    credentials
    default-credential
    set-credential
    update-credential
```