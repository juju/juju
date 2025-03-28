(command-juju-offers)=
# `juju offers`

```
Usage: juju offers [options] [<offer-name>]

Summary:
Lists shared endpoints.

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
--active-only  (= false)
    only return results where the offer is in use
--allowed-consumer (= "")
    return results where the user is allowed to consume the offer
--application (= "")
    return results matching the application
--connected-user (= "")
    return results where the user has a connection to the offer
--format  (= tabular)
    Specify output format (json|summary|tabular|yaml)
--interface (= "")
    return results matching the interface name
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-o, --output (= "")
    Specify an output file

Details:
List information about applications' endpoints that have been shared and who is connected.

The default tabular output shows each user connected (relating to) the offer, and the
relation id of the relation.

The summary output shows one row per offer, with a count of active/total relations.

The YAML output shows additional information about the source of connections, including
the source model UUID.

The output can be filtered by:
 - interface: the interface name of the endpoint
 - application: the name of the offered application
 - connected user: the name of a user who has a relation to the offer
 - allowed consumer: the name of a user allowed to consume the offer
 - active only: only show offers which are in use (are related to)

Examples:
    $ juju offers
    $ juju offers -m model
    $ juju offers --interface db2
    $ juju offers --application mysql
    $ juju offers --connected-user fred
    $ juju offers --allowed-consumer mary
    $ juju offers hosted-mysql
    $ juju offers hosted-mysql --active-only

See also:
   find-offers
   show-offer

Aliases: list-offers
```