(command-juju-show-status-log)=
# `juju show-status-log`

```
Usage: juju show-status-log [options] <entity name>

Summary:
Output past statuses for the specified entity.

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
--days  (= 0)
    Returns the logs for the past <days> days (cannot be combined with -n or --date)
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--from-date (= "")
    Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days)
--include-status-updates  (= false)
    Deprecated, has no effect for 2.3+ controllers: Include update status hook messages in the returned logs
-m, --model (= "")
    Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>
-n  (= 0)
    Returns the last N logs (cannot be combined with --days or --date)
-o, --output (= "")
    Specify an output file
--type (= "unit")
    Type of statuses to be displayed [application|container|juju-container|juju-machine|juju-unit|machine|model|saas|unit|workload]
--utc  (= false)
    Display time as UTC in RFC3339 format

Details:
This command will report the history of status changes for
a given entity.
The statuses are available for the following types.
-type supports:
    application:  statuses for the specified application
    container:  statuses from the agent that is managing containers
    juju-container:  statuses from the containers only and not their host machines
    juju-machine:  status of the agent that is managing a machine
    juju-unit:  statuses from the agent that is managing a unit
    machine:  statuses that occur due to provisioning of a machine
    model:  statuses for the model itself
    saas:  statuses for the specified SAAS application
    unit:  statuses for specified unit and its workload
    workload:  statuses for unit's workload

 and sorted by time of occurrence.
 The default is unit.
```