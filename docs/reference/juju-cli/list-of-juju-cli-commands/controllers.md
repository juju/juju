(command-juju-controllers)=
# `juju controllers`
> See also: [models](#models), [show-controller](#show-controller)

**Aliases:** list-controllers

## Summary
Lists all controllers.

## Usage
```juju controllers [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `--managed` | false | Specifies whether to show controllers managed by JAAS. |
| `-o`, `--output` |  | Specify an output file |
| `--refresh` | false | Specifies whether to connect to each controller to download the latest details. |

## Examples

    juju controllers
    juju controllers --format json --output ~/tmp/controllers.json



## Details
In the default tabular output, the current controller is marked with an asterisk.