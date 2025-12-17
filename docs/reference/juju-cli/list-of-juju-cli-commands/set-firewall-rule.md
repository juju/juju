(command-juju-set-firewall-rule)=
# `juju set-firewall-rule`
> See also: [firewall-rules](#firewall-rules)

## Summary
Sets a firewall rule.

## Usage
```juju set-firewall-rule [options] <service-name>, --allowlist <cidr>[,<cidr>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--allowlist` |  | Specifies a list of subnets to allowlist. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--whitelist` |  |  |

## Examples

    juju set-firewall-rule ssh --allowlist 192.168.1.0/16


## Details

Firewall rules control ingress to a well known services
within a Juju model. A rule consists of the service name
and a allowlist of allowed ingress subnets.
The currently supported services are:
- ssh
- juju-application-offer

DEPRECATION WARNING: 
Firewall rules have been moved to model configuration settings `ssh-allow` and
`saas-ingress-allow` This command is deprecated in favour of
reading/writing directly to these settings.