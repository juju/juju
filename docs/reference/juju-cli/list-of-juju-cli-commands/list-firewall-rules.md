(command-juju-list-firewall-rules)=
# `juju list-firewall-rules`

```
Usage: juju list-firewall-rules [options]

Summary:
Prints the firewall rules.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
Lists the firewall rules which control ingress to well known services
within a Juju model.

Examples:
    juju list-firewall-rules
    juju firewall-rules

See also:
    set-firewall-rule

Aliases: firewall-rules
```