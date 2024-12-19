(command-juju-controllers)=
# `juju controllers`
> See also: [models](#models), [show-controller](#show-controller)

## Summary
Lists all controllers.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--refresh` | false | Connect to each controller to download the latest details |

## Examples

    juju controllers
    juju controllers --format json --output ~/tmp/controllers.json



## Details
The output format may be selected with the '--format' option. In the
default tabular output, the current controller is marked with an asterisk.