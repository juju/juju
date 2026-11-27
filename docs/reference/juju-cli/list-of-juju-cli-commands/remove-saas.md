(command-juju-remove-saas)=
# `juju remove-saas`
> See also: [consume](#command-juju-consume), [offer](#command-juju-offer)

## Summary
Remove SAAS applications and their relations from the consuming model.

## Usage
```text
juju remove-saas [options] <saas-application-name> [<saas-application-name>...]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `--force` | false | Force the removal of the SAAS applications' relations |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-wait` | false | Requires --force; the wait for forced relation removal already defaults to zero |

## Examples

    juju remove-saas hosted-mysql
    juju remove-saas -m frontend hosted-mysql orders-db


## Details
Give the local names shown in the SAAS section of `juju status`, not
the offer URLs. On success the command prints nothing. To check whether
removal is complete, look for the local name in that model's SAAS section.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/saas-application/