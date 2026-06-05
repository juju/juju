(hook-command-status-get)=
# `status-get`
## Summary
Prints status information.

## Usage
``` status-get [options] [--include-data] [--application]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--application` | false | Prints status for all units of this application if this unit is the leader. |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `--include-data` | false | Prints all status data. |
| `-o`, `--output` |  | Specify an output file |

## Examples

    # Access the unit’s status:
    status-get
    status-get --include-data

    # Access the application’s status:
    status-get --application


## Details

`status-get` allows charms to query the current workload status.

Without arguments, it just prints the status code e.g. ‘maintenance’.
With `--include-data` specified, it prints YAML which contains the status
value plus any data associated with the status.

Include the `--application` option to get the overall status for the application,
rather than an individual unit.