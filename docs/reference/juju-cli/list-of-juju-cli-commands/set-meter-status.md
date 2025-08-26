(command-juju-set-meter-status)=
# `juju set-meter-status`
## Summary
Sets the meter status on an application or unit.

## Usage
```juju set-meter-status [options] [application or unit] status```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--info` |  | Set the meter status info to this string |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju set-meter-status myapp RED
    juju set-meter-status myapp/0 AMBER --info "my message"



## Details

Set meter status on the given application or unit. This command is used
to test the `meter-status-changed` hook for charms in development.