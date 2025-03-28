(hook-command-opened-ports)=
# `opened-ports`
## Summary
List all ports or port ranges opened by the unit.

## Usage
``` opened-ports [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` | false | display the list of target application endpoints for each port range |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |

## Examples

    opened-ports


## Details

opened-ports lists all ports or port ranges opened by a unit.

By default, the port range listing does not include information about the 
application endpoints that each port range applies to. Each list entry is
formatted as &lt;port&gt;/&lt;protocol&gt; (e.g. "80/tcp") or &lt;from&gt;-&lt;to&gt;/&lt;protocol&gt; 
(e.g. "8080-8088/udp").

If the --endpoints option is specified, each entry in the port list will be
augmented with a comma-delimited list of endpoints that the port range 
applies to (e.g. "80/tcp (endpoint1, endpoint2)"). If a port range applies to
all endpoints, this will be indicated by the presence of a '*' character
(e.g. "80/tcp (*)").

Opening ports is transactional (i.e. will take place on successfully exiting
the current hook), and therefore opened-ports will not return any values for
pending open-port operations run from within the same hook.