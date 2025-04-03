(command-juju-show-user)=
# `juju show-user`

```
Usage: juju show-user [options] [<user name>]

Summary:
Show information about a user.

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
-c, --controller (= "")
    Controller to operate in
--exact-time  (= false)
    Use full timestamp for connection times
--format  (= yaml)
    Specify output format (json|yaml)
-o, --output (= "")
    Specify an output file

Details:
By default, the YAML format is used and the user name is the current
user.


Examples:
    juju show-user
    juju show-user jsmith
    juju show-user --format json
    juju show-user --format yaml

See also:
    add-user
    register
    users
```