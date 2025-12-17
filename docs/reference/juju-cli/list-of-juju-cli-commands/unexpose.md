(command-juju-unexpose)=
# `juju unexpose`
> See also: [expose](#expose)

## Summary
Removes public availability over the network for an application.

## Usage
```juju unexpose [options] <application name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--endpoints` |  | Specifies (in comma-delimited format) the ports opened by the charm which should now be unexposed. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju unexpose apache2

To unexpose only the ports that charms have opened for the "www", or "www" and "logs" endpoints:

    juju unexpose apache2 --endpoints www

    juju unexpose apache2 --endpoints www,logs


## Details

Adjusts the firewall rules and any relevant security mechanisms of the
cloud to deny public access to the application.

Applications are unexposed by default when they get created. If exposed via
the `juju expose` command, they can be unexposed by running the `juju unexpose`
command.

If no additional options are specified, the command will unexpose the
application (if exposed).

The `--endpoints` option may be used to restrict the effect of this command to
the list of ports opened for a comma-delimited list of endpoints.

Note that when the `--endpoints`option is provided, the application will still
remain exposed if any other of its endpoints are still exposed. However, if
none of its endpoints remain exposed, the application will become unexposed.