(command-juju-whoami)=
# `juju whoami`
> See also: [controllers](#command-juju-controllers), [login](#command-juju-login), [logout](#command-juju-logout), [models](#command-juju-models), [users](#command-juju-users)

## Summary
Print current login details.

## Usage
```text
juju whoami [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju whoami


## Details
Display the current controller, model and logged in user name.