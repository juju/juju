(command-juju-credentials)=
# `juju credentials`

```
Usage: juju credentials [options] [<cloud name>]

Summary:
Lists Juju credentials for a cloud.

Global Options:
--debug  (= false)
    equivalent to --show-log --logging-config=<root>=DEBUG
-h, --help  (= false)
    Show help on a command or other topic.
--logging-config (= "")
    specify log levels for modules
--quiet  (= false)
    show no informational output
--show-log  (= false)
    if set, write the log file to stderr
--verbose  (= false)
    show more verbose output

Command Options:
-B, --no-browser-login  (= false)
    Do not use web browser for authentication
-c, --controller (= "")
    Controller to operate in
--client  (= false)
    Client operation
--format  (= tabular)
    Specify output format (json|tabular|yaml)
--local  (= false)
    DEPRECATED (use --client): Local operation only; controller not affected
-o, --output (= "")
    Specify an output file
--show-secrets  (= false)
    Show secrets, applicable to yaml or json formats only

Details:
This command list credentials from this client and credentials
from a controller.

Locally stored credentials are client specific and
are used with `juju bootstrap`
and `juju add-model`. It's paramount to understand that
different client devices may have different locally stored credentials
for the same user.

Remotely stored credentials or controller stored credentials are
stored on the controller.

An arbitrary "credential name" is used to represent credentials, which are
added either via `juju add-credential` or `juju autoload-credentials`.
Note that there can be multiple sets of credentials and, thus, multiple
names.

Actual authentication material is exposed with the '--show-secrets'
option in json or yaml formats. Secrets are not shown in tabular format.

A controller, and subsequently created models, can be created with a
different set of credentials but any action taken within the model (e.g.:
`juju deploy`; `juju add-unit`) applies the credential used
to create that model. This model credential is stored on the controller.

A credential for 'controller' model is determined at bootstrap time and
will be stored on the controller. It is considered to be controller default.

Recall that when a controller is created a 'default' model is also
created. This model will use the controller default credential.
To see details of your credentials use "juju show-credential" command.

When adding a new model, Juju will reuse the controller default credential.
To add a model that uses a different credential, specify a  credential
from this client using --credential option. See `juju help add-model`
for more information.

Credentials denoted with an asterisk '*' are currently set as the user default
for a given cloud.

Use --controller option to list credentials from a controller.

Use --client option to list credentials known locally on this client.

Examples:
    juju credentials
    juju credentials aws
    juju credentials aws --client
    juju credentials --format yaml --show-secrets
    juju credentials --controller mycontroller
    juju credentials --controller mycontroller --client

See also:
    add-credential
    update-credential
    remove-credential
    default-credential
    autoload-credentials
    show-credentials

Aliases: list-credentials
```