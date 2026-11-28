(command-juju-set-credential)=
# `juju set-credential`
> See also: [credentials](#command-juju-credentials), [show-credential](#command-juju-show-credential), [update-credential](#command-juju-update-credential)

## Summary
Changes the credential used by one model.

## Usage
```text
juju set-credential [options] <cloud name> <credential name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-m`, `--model` |  | Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt; |

## Examples

Switch the model trinity to the AWS credential named bob:

    juju set-credential -m trinity aws bob


## Details

Choose a credential for the model's cloud under the signed-in account.
The CLI uses a matching controller copy first. If the lookup fails or
finds no match, it tries to upload the local copy before requesting
the switch. A failed lookup does not mean the name is unused: the upload
can replace the material for every model using that cloud, owner and name,
even if the controller refuses the switch. Check both copies before
running this command. To change only one model, use a separately named
credential whose contents you have verified.

The controller requires model admin or controller superuser access and
refuses the switch while model changes are blocked. Use
`juju update-credential` to replace material shared by several models.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/model/