(command-juju-status)=
# `juju status`
> See also: [machines](#machines), [show-model](#show-model), [show-status-log](#show-status-log), [storage](#storage)

## Summary
Report the status of the model, its machines, applications and units.

## Usage
```juju status [options] [<selector> [...]]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Use ANSI color codes in tabular output |
| `--format` | tabular | Specify output format (json&#x7c;line&#x7c;oneline&#x7c;short&#x7c;summary&#x7c;tabular&#x7c;yaml) |
| `--integrations` | false | Same as `--relations` |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-color` | false | Disable ANSI color codes in tabular output |
| `-o`, `--output` |  | Specify an output file |
| `--relations` | false | Show relations section in tabular output |
| `--retry-count` | 3 | Number of times to retry API failures |
| `--retry-delay` | 100ms | Time to wait between retry attempts |
| `--storage` | false | Show storage section in tabular output |
| `--utc` | false | Display timestamps in the UTC timezone |
| `--watch` | 0s | Watch the status every period of time |

## Examples

Report the status of units hosted on machine `0`:

    juju status 0

Report the status of the `mysql` application:

    juju status mysql

Report the status for applications that start with `nova-`:

    juju status nova-*

Include information about storage and relations in output:

    juju status --storage --relations

Provide output as valid `JSON`:

    juju status --format=json

Watch the status every five seconds:

    juju status --watch 5s

Show only applications/units in active status:

    juju status active

Show only applications/units in error status:

    juju status error


## Details

Report the model's status, optionally filtered by names of applications or
units. When selectors are present, filter the report to exclude entities that
do not match.

    juju status [<selector> [...]]

`<selector>` selects machines, units or applications from the model to display.
Wildcard characters (`*`) enable multiple entities to be matched at the same
time.

    (<machine>|<unit>|<application>)[*]

When an entity that matches &lt;selector&gt; is integrated with other applications, the
status of those applications will also be presented. By default (without a
`<selector>`) the status of all applications and their units will be displayed.


### Altering the output format

The `--format` option allows you to specify how the status report is formatted.

- `--format=tabular` (default):
Displays information about all aspects of the model in a human-centric manner.
Omits some information by default.
Use the `--relations` and `--storage` options to include all available information.
- `--format=line`, `--format=short`, `--format=oneline `:
Reports information from units. Includes their IP address, open ports and the status of the workload and agent.
- `--format=summary`:
Reports aggregated information about the model. Includes a description of subnets and ports that are in use,
the counts of applications, units, and machines by status code.
- `--format=json`, `--format=yaml`:
Provides information in a `JSON` or `YAML` format for programmatic use.