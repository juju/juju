(command-juju-show-action)=
# `juju show-action`
> See also: [actions](#actions), [run](#run)

## Summary
Shows detailed information about an action.

## Usage
```juju show-action [options] <application> <action>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju show-action postgresql backup


## Details

Shows detailed information about an action on the target application.