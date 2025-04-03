(command-juju-retry-provisioning)=
# `juju retry-provisioning`

```
Usage: juju retry-provisioning [options] <machine> [...]

Summary:
Retries provisioning for failed machines.

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
--all  (= false)
    retry provisioning all failed machines
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
```