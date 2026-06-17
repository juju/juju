(command-juju-retry-provisioning)=
# `juju retry-provisioning`
## Summary
Retries provisioning for failed machines.

## Usage
```juju retry-provisioning [options] <machine> [...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Retry provisioning all failed machines |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples


	juju retry-provisioning 0

	juju retry-provisioning 0 1

	juju retry-provisioning --all