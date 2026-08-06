(command-juju-enable-destroy-controller)=
# `juju enable-destroy-controller`
> See also: [disable-command](#command-juju-disable-command), [disabled-commands](#command-juju-disabled-commands), [enable-command](#command-juju-enable-command)

## Summary
Enable destroy-controller by removing disabled commands in the controller.

## Usage
```text
juju enable-destroy-controller [options]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Details

Any model in the controller that has disabled commands will block a controller
from being destroyed.

A controller administrator can enable all the commands across all the models
in a Juju controller so that the controller can be destroyed if desired.