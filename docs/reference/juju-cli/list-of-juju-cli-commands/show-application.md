(command-juju-show-application)=
# `juju show-application`
## Summary
Displays information about an application.

## Usage
```juju show-application [options] <application name or alias>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--format` | yaml | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-application mysql
    juju show-application mysql wordpress

    juju show-application myapplication

where `myapplication` is the application name alias; see `juju help deploy` for more information.


## Details

Takes deployed application names or aliases as an argument.

The command performs an exact search. It does not support wildcards.