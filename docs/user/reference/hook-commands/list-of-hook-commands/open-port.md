(hook-command-open-port)=
# `open-port`

## Summary
Register a request to open a port or port range.

## Usage
``` open-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` |  | a comma-delimited list of application endpoints to target with this operation |
| `--format` |  | deprecated format flag |

## Examples

    # Open port 80 to TCP traffic:
    open-port 80/tcp

    # Open port 1234 to UDP traffic:
    open-port 1234/udp

    # Open a range of ports to UDP traffic:
    open-port 1000-2000/udp

    # Open a range of ports to TCP traffic for specific
    # application endpoints (since Juju 2.9):
    open-port 1000-2000/tcp --endpoints dmz,monitoring


## Details

open-port registers a request to open the specified port or port range.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.

The behavior differs a little bit between machine charms and Kubernetes charms.

Machine charms
On public clouds the port will only be open while the application is exposed.
It accepts a single port or range of ports with an optional protocol, which
may be icmp, udp, or tcp. tcp is the default.

open-port will not have any effect if the application is not exposed, and may
have a somewhat delayed effect even if it is. This operation is transactional,
so changes will not be made unless the hook exits successfully.

Prior to Juju 2.9, when charms requested a particular port range to be opened,
Juju would automatically mark that port range as opened for all defined
application endpoints. As of Juju 2.9, charms can constrain opened port ranges
to a set of application endpoints by providing the --endpoints flag followed by
a comma-delimited list of application endpoints.

Kubernetes charms
The port will open directly regardless of whether the application is exposed or not.
This connects to the fact that juju expose currently has no effect on sidecar charms.
Additionally, it is currently not possible to designate a range of ports to open for
Kubernetes charms; to open a range, you will have to run open-port multiple times.