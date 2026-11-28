(command-juju-model-secret-backend)=
# `juju model-secret-backend`
> See also: [add-secret-backend](#command-juju-add-secret-backend), [secret-backends](#command-juju-secret-backends), [remove-secret-backend](#command-juju-remove-secret-backend), [show-secret-backend](#command-juju-show-secret-backend), [update-secret-backend](#command-juju-update-secret-backend)

## Summary
Displays or sets the secret backend for a model.

## Usage
```text
juju model-secret-backend [options] [<secret-backend-name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Display the secret backend for the current model:

    juju model-secret-backend

Select an existing backend or restore this model's default:

    juju model-secret-backend myvault
    juju model-secret-backend auto


## Details

With no argument, prints the selected backend name or auto. To change it,
pass the exact name of a registered backend, or auto for the model default.
A successful change prints nothing. Read back the selection with this command.
To see the recorded backend for each revision of a secret, run
`juju show-secret NAME --revisions`.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/secret-backend/