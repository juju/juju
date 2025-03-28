(command-juju-list-spaces)=
# `juju list-spaces`
> See also: [add-space](#add-space), [reload-spaces](#reload-spaces)

**Aliases:** list-spaces

## Summary
List known spaces, including associated subnets.

## Usage
```juju spaces [options] [--short] [--format yaml|json] [--output <path>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--short` | false | only display spaces. |

## Examples

List spaces and their subnets:

	juju spaces

List spaces:

	juju spaces --short


## Details
Displays all defined spaces. By default both spaces and their subnets are displayed.
Supplying the --short option will list just the space names.
The --output argument allows the command's output to be redirected to a file.