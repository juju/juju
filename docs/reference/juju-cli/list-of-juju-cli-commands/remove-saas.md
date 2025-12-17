(command-juju-remove-saas)=
# `juju remove-saas`
> See also: [consume](#consume), [offer](#offer)

## Summary
Removes consumed applications (SAAS) from the model.

## Usage
```juju remove-saas [options] <saas-application-name> [<saas-application-name>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `--force` | false | Completely removes a SAAS and all its dependencies. |
| `-m`, `--model` |  | Specifies the model to operate in. Accepts `[<controller name>:]<model name>|<model UUID>`. |
| `--no-wait` | false | Rushes through SAAS removal without waiting for each individual step to complete. |

## Examples

    juju remove-saas hosted-mysql
    juju remove-saas -m test-model hosted-mariadb



## Details
Removes a consumed (SAAS) application and terminates any relations that the
application has, potentially leaving any related local applications
in a non-functional state.