(command-juju-enable-destroy-controller)=
# `juju enable-destroy-controller`
> See also: [disable-command](#disable-command), [disabled-commands](#disabled-commands), [enable-command](#enable-command)

## Summary
Enable destroy-controller by removing disabled commands in the controller.

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |

## Details

Any model in the controller that has disabled commands will block a controller
from being destroyed.

A controller administrator is able to enable all the commands across all the models
in a Juju controller so that the controller can be destoyed if desired.