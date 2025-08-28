(command-juju-subnets)=
# `juju subnets`
**Aliases:** list-subnets

## Summary
List subnets known to Juju.

## Usage
```juju subnets [options] [--space <name>] [--zone <name>] [--format yaml|json] [--output <path>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |
| `--space` |  | Filter results by space name |
| `--zone` |  | Filter results by zone name |

## Examples

To list all subnets known to Juju:

    juju subnets

To list subnets associated with a specific network space:

    juju subnets --space my-space

To list subnets associated with a specific availability zone:

    juju subnets --zone my-zone


## Details
Displays a list of all subnets known to Juju. Results can be filtered
using the optional --space and/or --zone arguments to only display
subnets associated with a given network space and/or availability zone.

Like with other Juju commands, the output and its format can be changed
using the `--format` and `--output` (or `-o`) optional arguments. Supported
output formats include `yaml` (default) and `json`. To redirect the
output to a file, use `--output`.