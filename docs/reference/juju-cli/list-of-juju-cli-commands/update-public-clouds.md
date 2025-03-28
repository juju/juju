(command-juju-update-public-clouds)=
# `juju update-public-clouds`

```
Usage: juju update-public-clouds [options]

Summary:
Updates public cloud information available to Juju.

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
If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

Use --controller option to update public cloud(s) on a controller. The command
will only update the clouds that a controller knows about.

Use --client to update a definition of public cloud(s) on this client.

Examples:

    juju update-public-clouds
    juju update-public-clouds --client
    juju update-public-clouds --controller mycontroller

See also:
    clouds
```