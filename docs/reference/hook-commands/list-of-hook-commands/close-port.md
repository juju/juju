(hook-command-close-port)=
# `close-port`

## Summary
Register a request to close a port or port range.

## Usage
``` close-port [options] <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--endpoints` |  | a comma-delimited list of application endpoints to target with this operation |
| `--format` |  | deprecated format flag |

## Examples

    # Close single port
    close-port 80

    # Close a range of ports
    close-port 9000-9999/udp

    # Disable ICMP
    close-port icmp

    # Close a range of ports for a set of endpoints (since Juju 2.9)
    close-port 80-90 --endpoints dmz,public


## Details

close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.