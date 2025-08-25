(command-juju-list-machines)=
# `juju list-machines`
> See also: [status](#status)

**Aliases:** list-machines

## Summary
Lists machines in a model.

## Usage
```juju machines [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--color` | false | Force use of ANSI color codes |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--utc` | false | Display time as UTC in RFC3339 format |

## Examples

     juju machines


## Details

By default, the tabular format is used.
The following sections are included: `ID`, `STATE`, `DNS`, `INS-ID`, `SERIES`, `AZ`
Note: Above, `AZ` is the cloud region's availability zone.