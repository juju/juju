(command-juju-machines)=
# `juju machines`
> See also: [status](#status)

**Aliases:** list-machines

## Summary
Lists machines in a model.

## Usage
```juju machines [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--color` | false | Specifies whether to force use of ANSI color codes. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Specifies whether to display time as UTC in RFC3339 format. |

## Examples

     juju machines


## Details

Uses the tabular format by default.
The following sections are included: `ID`, `STATE`, `DNS`, `INS-ID`, `SERIES`, `AZ`
Note: Above, `AZ` is the cloud region's availability zone.