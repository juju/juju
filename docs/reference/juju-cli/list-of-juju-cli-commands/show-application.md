(command-juju-show-application)=
# `juju show-application`
## Summary
Displays information about an application.

## Usage
```juju show-application [options] <application name or alias>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--format` | yaml | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju show-application mysql
    juju show-application mysql wordpress

    juju show-application myapplication

where `myapplication` is the application name alias; see `juju help deploy` for more information.


## Details

The command takes deployed application names or aliases as an argument.

The command does an exact search. It does not support wildcards.