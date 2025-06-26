(command-juju-show-status-log)=
# `juju show-status-log`
> See also: [status](#status)

## Summary
Output past statuses for the specified entity.

## Usage
```juju show-status-log [options] <entity name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--days` | 0 | Returns the logs for the past &lt;days&gt; days (cannot be combined with -n or --date) |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--from-date` |  | Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-n` | 0 | Returns the last N logs (cannot be combined with --days or --date) |
| `-o`, `--output` |  | Specify an output file |
| `--type` | unit | Type of statuses to be displayed [application&#x7c;container&#x7c;filesystem&#x7c;juju-container&#x7c;juju-machine&#x7c;juju-unit&#x7c;machine&#x7c;model&#x7c;saas&#x7c;unit&#x7c;volume&#x7c;workload] |
| `--utc` | false | Display time as UTC in RFC3339 format |

## Examples

Show the status history for the specified unit:

    juju show-status-log mysql/0

Show the status history for the specified unit with the last 30 logs:

    juju show-status-log mysql/0 -n 30

Show the status history for the specified unit with the logs for the past 2 days:

    juju show-status-log mysql/0 -days 2

Show the status history for the specified unit with the logs for any date after 2020-01-01:

    juju show-status-log mysql/0 --from-date 2020-01-01

Show the status history for the specified application:

    juju show-status-log -type application wordpress

Show the status history for the specified machine:

    juju show-status-log 0

Show the status history for the model:

    juju show-status-log -type model


## Details

This command will report the history of status changes for
a given entity.
The statuses are available for the following types.
-type supports:
    application:  statuses for the specified application
    container:  statuses from the agent that is managing containers
    filesystem:  statuses from the specified filesystem
    juju-container:  statuses from the containers only and not their host machines
    juju-machine:  status of the agent that is managing a machine
    juju-unit:  statuses from the agent that is managing a unit
    machine:  statuses that occur due to provisioning of a machine
    model:  statuses for the model itself
    saas:  statuses for the specified SAAS application
    unit:  statuses for specified unit and its workload
    volume:  statuses from the specified volume
    workload:  statuses for unit's workload

 and sorted by time of occurrence.
 The default is unit.