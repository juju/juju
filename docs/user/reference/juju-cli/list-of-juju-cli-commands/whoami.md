(command-juju-whoami)=
# `juju whoami`
> See also: [controllers](#controllers), [login](#login), [logout](#logout), [models](#models), [users](#users)

## Summary
Print current login details.

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