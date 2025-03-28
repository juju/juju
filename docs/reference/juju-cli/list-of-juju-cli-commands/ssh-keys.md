(command-juju-ssh-keys)=
# `juju ssh-keys`

```
Usage: juju ssh-keys [options]

Summary:
Lists the currently known SSH keys for the current (or specified) model.

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
--full  (= false)
    Show full key instead of just the fingerprint
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>

Details:
Juju maintains a per-model cache of SSH keys which it copies to each newly
created unit.
This command will display a list of all the keys currently used by Juju in
the current model (or the model specified, if the '-m' option is used).
By default a minimal list is returned, showing only the fingerprint of
each key and its text identifier. By using the '--full' option, the entire
key may be displayed.

Examples:
    juju ssh-keys

To examine the full key, use the '--full' option:

    juju ssh-keys -m jujutest --full

Aliases: list-ssh-keys
```