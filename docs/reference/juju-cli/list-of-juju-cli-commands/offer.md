(command-juju-offer)=
# `juju offer`

```
Usage: juju offer [options] [model-name.]<application-name>:<endpoint-name>[,...] [offer-name]

Summary:
Offer application endpoints for use in other models.

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

Details:
Deployed application endpoints are offered for use by consumers.
By default, the offer is named after the application, unless
an offer name is explicitly specified.

Examples:

$ juju offer mysql:db
$ juju offer mymodel.mysql:db
$ juju offer db2:db hosted-db2
$ juju offer db2:db,log hosted-db2

See also:
    consume
    relate
    remove-saas
```