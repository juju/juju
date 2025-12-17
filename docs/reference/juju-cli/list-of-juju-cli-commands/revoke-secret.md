(command-juju-revoke-secret)=
# `juju revoke-secret`
## Summary
Revokes access to a secret.

## Usage
```juju revoke-secret [options] <ID>|<name> <application>[,<application>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |

## Examples

    juju revoke-secret my-secret ubuntu-k8s
    juju revoke-secret 9m4e2mr0ui3e8a215n4g ubuntu-k8s,prometheus-k8s


## Details

Revokes applications' access to view the value of a specified secret.