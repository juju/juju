(command-juju-model-secret-backend)=
# `juju model-secret-backend`
> See also: [add-secret-backend](#add-secret-backend), [secret-backends](#secret-backends), [remove-secret-backend](#remove-secret-backend), [show-secret-backend](#show-secret-backend), [update-secret-backend](#update-secret-backend)

## Summary
Displays or sets the secret backend for a model.

## Usage
```juju model-secret-backend [options] [<secret-backend-name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Display the secret backend for the current model:

   juju model-secret-backend

Set the secret backend to myVault for the current model:

   juju model-secret-backend myVault


## Details

Sets or displays the secret backend for the current model.