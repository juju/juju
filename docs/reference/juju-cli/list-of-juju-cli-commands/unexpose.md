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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--endpoints` |  | Unexpose only the ports that charms have opened for this comma-delimited list of endpoints |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

    juju unexpose apache2

To unexpose only the ports that charms have opened for the "www", or "www" and "logs" endpoints:

    juju unexpose apache2 --endpoints www

    juju unexpose apache2 --endpoints www,logs


## Details
Adjusts the firewall rules and any relevant security mechanisms of the
cloud to deny public access to the application.

Applications are unexposed by default when they get created. If exposed via
the "juju expose" command, they can be unexposed by running the "juju unexpose"
command.  

If no additional options are specified, the command will unexpose the
application (if exposed).

The --endpoints option may be used to restrict the effect of this command to 
the list of ports opened for a comma-delimited list of endpoints.

Note that when the --endpoints option is provided, the application will still
remain exposed if any other of its endpoints are still exposed. However, if
none of its endpoints remain exposed, the application will be instead unexposed.