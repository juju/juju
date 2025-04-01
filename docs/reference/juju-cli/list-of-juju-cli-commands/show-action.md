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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju show-action postgresql backup


## Details

Show detailed information about an action on the target application.