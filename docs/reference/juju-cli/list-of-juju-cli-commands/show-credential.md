(command-juju-show-credential)=
# `juju show-credential`
> See also: [credentials](#command-juju-credentials), [add-credential](#command-juju-add-credential), [update-credential](#command-juju-update-credential), [remove-credential](#command-juju-remove-credential), [autoload-credentials](#command-juju-autoload-credentials)

**Aliases:** show-credentials

## Summary
Shows credential information stored either on this client or on a controller.

## Usage
```text
juju show-credential [options] [<cloud name> <credential name>]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--format` | yaml | Specify output format (yaml) |
| `-o`, `--output` |  | Specify an output file |
| `--show-secrets` | false | Display credential secret attributes |

## Examples

Inspect the AWS controller credential named production:

    juju show-credential aws production --controller mycontroller

Compare the client and controller copies of a named AWS credential:

    juju show-credential aws production --client --controller mycontroller

Inspect all credentials on this client:

    juju show-credential --client

Inspect all credentials under your account on mycontroller:

    juju show-credential --controller mycontroller

Inspect a named AWS credential with hidden values on a private terminal:

    juju show-credential aws production --controller mycontroller --show-secrets


## Details
Display credential contents as YAML. Supply the cloud and credential
name to inspect one credential, or omit both to inspect all credentials
in the selected stores.

By default, read the client store and the signed-in account on the
current controller, if there is one. Use `--client` for this
client, `--controller NAME` for the named controller, or both to
compare copies. Controller output includes stored validity and can list
models you can access that use the credential.

Hidden values are omitted by default. To see them, use
`--show-secrets` with a named credential and keep the output private.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/