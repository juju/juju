(command-juju-version)=
# `juju version`
> See also: [show-controller](#show-controller), [show-model](#show-model)

## Summary
Prints the Juju CLI client version.

## Usage
```juju version [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Specifies whether to print all version information. |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju version

Print all version information:

    juju version --all


## Details

Prints only the `juju `CLI client version.