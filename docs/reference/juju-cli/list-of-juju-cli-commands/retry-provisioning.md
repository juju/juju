(command-juju-retry-provisioning)=
# `juju retry-provisioning`
## Summary
Retries provisioning for failed machines.

## Usage
```juju retry-provisioning [options] <machine> [...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--all` | false | Specifies whether to retry provisioning for all failed machines. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples


	juju retry-provisioning 0

	juju retry-provisioning 0 1

	juju retry-provisioning --all