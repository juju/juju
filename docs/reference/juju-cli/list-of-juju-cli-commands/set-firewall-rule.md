(command-juju-set-firewall-rule)=
# `juju set-firewall-rule`

```
Usage: juju set-firewall-rule [options] <service-name>, --whitelist <cidr>[,<cidr>...]

Summary:
Sets a firewall rule.

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
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
--whitelist (= "")
    list of subnets to whitelist

Details:
Firewall rules control ingress to a well known services
within a Juju model. A rule consists of the service name
and a whitelist of allowed ingress subnets.
The currently supported services are:
 -ssh

Examples:
    juju set-firewall-rule ssh --whitelist 192.168.1.0/16

See also:
    list-firewall-rules
```