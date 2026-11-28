(command-juju-credentials)=
# `juju credentials`
> See also: [add-credential](#command-juju-add-credential), [update-credential](#command-juju-update-credential), [remove-credential](#command-juju-remove-credential), [default-credential](#command-juju-default-credential), [autoload-credentials](#command-juju-autoload-credentials), [show-credential](#command-juju-show-credential)

**Aliases:** list-credentials

## Summary
Lists Juju credentials for a cloud.

## Usage
```text
juju credentials [options] [<cloud name>]
```

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

List credentials from the available stores:

    juju credentials

List local credentials for AWS:

    juju credentials aws --client

List your credentials on a controller:

    juju credentials --controller mycontroller

Include the local list alongside a named controller:

    juju credentials --controller mycontroller --client


## Details

List credentials saved on this client, controller credentials belonging to
your current account, or both. A cloud argument filters the local list, not
the controller list. The asterisk marks this client's default for a cloud;
it does not identify a controller default.

Tabular output does not show secret material. Requesting secrets in YAML
or JSON can disclose cloud account credentials. Protect the output.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/