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
| `--allowlist` |  | list of subnets to allowlist |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
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

DEPRECATION WARNING: Firewall rules have been moved to model-config settings "ssh-allow" and
"saas-ingress-allow". This command is deprecated in favour of
reading/writing directly to these settings.