(command-juju-firewall-rules)=
# `juju firewall-rules`
> See also: [set-firewall-rule](#set-firewall-rule)

**Aliases:** list-firewall-rules

## Summary
Prints the firewall rules.

## Usage
```juju firewall-rules [options] ```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `-o`, `--output` |  | Specify an output file |

## Examples

    juju firewall-rules



## Details

Lists the firewall rules which control ingress to well known services
within a Juju model.

DEPRECATION WARNING: Firewall rules have been moved to model-config settings "ssh-allow" and
"saas-ingress-allow". This command is deprecated in favour of
reading/writing directly to these settings.