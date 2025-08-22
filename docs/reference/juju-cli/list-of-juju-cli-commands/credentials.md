(command-juju-credentials)=
# `juju credentials`
> See also: [add-credential](#add-credential), [update-credential](#update-credential), [remove-credential](#remove-credential), [default-credential](#default-credential), [autoload-credentials](#autoload-credentials), [show-credential](#show-credential)

**Aliases:** list-credentials

## Summary
Lists Juju credentials for a cloud.

## Usage
```juju credentials [options] [<cloud name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--format` | tabular | Specify output format (json&#x7c;tabular&#x7c;yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--show-secrets` | false | Show secrets, applicable to yaml or json formats only |

## Examples

    juju credentials
    juju credentials aws
    juju credentials aws --client
    juju credentials --format yaml --show-secrets
    juju credentials --controller mycontroller
    juju credentials --controller mycontroller --client


## Details
This command list credentials from this client and credentials
from a controller.

Locally stored credentials are client specific and
are used with `juju bootstrap`
and `juju add-model`. It's paramount to understand that
different client devices may have different locally stored credentials
for the same user.

Remotely stored credentials or controller stored credentials are
stored on the controller.

An arbitrary credential name is used to represent credentials, which are
added either via `juju add-credential` or `juju autoload-credentials`.
Note that there can be multiple sets of credentials and, thus, multiple
names.

Actual authentication material is exposed with the `--show-secrets`
option in json or yaml formats. Secrets are not shown in tabular format.

A controller, and subsequently created models, can be created with a
different set of credentials but any action taken within the model (e.g.:
`juju deploy`; `juju add-unit`) applies the credential used
to create that model. This model credential is stored on the controller.

A credential for 'controller' model is determined at bootstrap time and
will be stored on the controller. It is considered to be controller default.

When adding a new model, Juju will reuse the controller default credential.
To add a model that uses a different credential, specify a  credential
from this client using the `--credential` option. See `juju help add-model`
for more information.

Credentials denoted with an asterisk `*` are currently set as the user default
for a given cloud.

Use the `--controller` option to list credentials from a controller.

Use `--client` option to list credentials known locally on this client.