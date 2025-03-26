(command-juju-show-unit)=
# `juju show-unit`
> See also: [add-unit](#add-unit), [remove-unit](#remove-unit)

## Summary
Displays information about a unit.

## Usage
```juju show-unit [options] <unit name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--app` | false | only show application relation data |
| `--endpoint` |  | only show relation data for the specified endpoint |
| `--format` | yaml | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--related-unit` |  | only show relation data for the specified unit |

## Examples

To show information about a unit:

    juju show-unit mysql/0

To show information about multiple units:

    juju show-unit mysql/0 wordpress/1

To show only the application relation data for a unit:

    juju show-unit mysql/0 --app

To show only the relation data for a specific endpoint:

    juju show-unit mysql/0 --endpoint db

To show only the relation data for a specific related unit:

    juju show-unit mysql/0 --related-unit wordpress/2


## Details

The command takes deployed unit names as an argument.

Optionally, relation data for only a specified endpoint
or related unit may be shown, or just the application data.