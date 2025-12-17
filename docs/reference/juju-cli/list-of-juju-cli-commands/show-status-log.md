(command-juju-show-status-log)=
# `juju show-status-log`
**Aliases:** status-history

## Summary
Outputs past statuses for the specified entity.

## Usage
```juju show-status-log [options] <entity> ...```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--days` | 0 | Returns the logs for the past &lt;days&gt; days (cannot be combined with -n or --date). |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--from-date` |  | Returns logs for any date after the passed one, the expected date format is YYYY-MM-DD (cannot be combined with -n or --days). |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-n` | 0 | Returns the last N logs (cannot be combined with --days or --date). |
| `-o`, `--output` |  | Specify an output file |
| `--type` | unit | Specifies the type of statuses to be displayed [application&#x7c;container&#x7c;juju-container&#x7c;juju-machine&#x7c;juju-unit&#x7c;machine&#x7c;model&#x7c;saas&#x7c;unit&#x7c;workload] |
| `--utc` | false | Specifies whether to display time as UTC in RFC3339 format. |

## Details

Reports the history of status information for a given entity.