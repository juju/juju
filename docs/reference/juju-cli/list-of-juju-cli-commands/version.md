(command-juju-version)=
# `juju version`
> See also: [show-controller](#show-controller), [show-model](#show-model)

## Summary
Print the Juju CLI client version.

## Usage
```juju version [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Prints all version information |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju version

Print all version information:

    juju version --all


## Details

Print only the `juju `CLI client version.