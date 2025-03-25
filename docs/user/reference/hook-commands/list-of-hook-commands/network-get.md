(hook-command-network-get)=
# `network-get`
## Summary
Get network config.

## Usage
``` network-get [options] <binding-name> [--ingress-address] [--bind-address] [--egress-subnets]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--bind-address` | false | get the address for the binding on which the unit should listen |
| `--egress-subnets` | false | get the egress subnets for the binding |
| `--format` | smart | Specify output format (json&#x7c;smart&#x7c;yaml) |
| `--ingress-address` | false | get the ingress address for the binding |
| `-o`, `--output` |  | Specify an output file |
| `--primary-address` | false | (deprecated) get the primary address for the binding |
| `-r`, `--relation` |  | specify a relation by id |

## Examples

    network-get dbserver
    network-get dbserver --bind-address

    See https://discourse.charmhub.io/t/charm-network-primitives/1126 for more
    in depth examples and explanation of usage.


## Details

network-get returns the network config for a given binding name. By default
it returns the list of interfaces and associated addresses in the space for
the binding, as well as the ingress address for the binding. If defined, any
egress subnets are also returned.
If one of the following flags are specified, just that value is returned.
If more than one flag is specified, a map of values is returned.

    --bind-address: the address the local unit should listen on to serve connections, as well
                    as the address that should be advertised to its peers.
    --ingress-address: the address the local unit should advertise as being used for incoming connections.
    --egress-subnets: subnets (in CIDR notation) from which traffic on this relation will originate.