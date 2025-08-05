> See also: [credentials](#credentials), [show-credential](#show-credential), [update-credential](#update-credential)

## Summary
Relates a remote credential to a model.

## Usage
```juju set-credential [options] <cloud name> <credential name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

For cloud 'aws', relate remote credential 'bob' to model 'trinity':

    juju set-credential -m trinity aws bob


## Details

This command relates a credential cached on a controller to a specific model.
It does not change/update the contents of an existing active credential. See
command `update-credential` for that.

The credential specified may exist locally (on the client), remotely (on the
controller), or both. The command will error out if the credential is stored
neither remotely nor locally.

When remote, the credential will be related to the specified model.

When local and not remote, the credential will first be uploaded to the
controller and then related.

This command does not affect an existing relation between the specified
credential and another model. If the credential is already related to a model
this operation will result in that credential being related to two models.

Use the `show-credential` command to see how remote credentials are related
to models.



