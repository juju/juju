(command-juju-remove-saas)=
# `juju remove-saas`
> See also: [consume](#consume), [offer](#offer)

## Summary
Remove consumed applications (SAAS) from the model.

## Usage
```juju remove-saas [options] <saas-application-name> [<saas-application-name>...]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `--force` | false | Completely removes a SAAS and all its dependencies. |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |
| `--no-wait` | false | Rushes through SAAS removal without waiting for each individual step to complete. |

## Examples

    juju remove-saas hosted-mysql
    juju remove-saas -m test-model hosted-mariadb



## Details
Remove a consumed (SAAS) application, terminating any relations that the
application has, potentially leaving any related local applications
in a non-functional state.