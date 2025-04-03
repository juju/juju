(command-juju-find-offers)=
# `juju find-offers`

```
Usage: juju find-offers [options]

Summary:
Find offered application endpoints.

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
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--interface (= "")
    return results matching the interface name
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file
--offer (= "")
    return results matching the offer name
--url (= "")
    return results matching the offer URL

Details:
Find which offered application endpoints are available to the current user.

This command is aimed for a user who wants to discover what endpoints are available to them.

Examples:
   $ juju find-offers
   $ juju find-offers mycontroller:
   $ juju find-offers fred/prod
   $ juju find-offers --interface mysql
   $ juju find-offers --url fred/prod.db2
   $ juju find-offers --offer db2

See also:
   show-offer
```