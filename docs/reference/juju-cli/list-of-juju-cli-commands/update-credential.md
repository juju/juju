(command-juju-update-credential)=
# `juju update-credential`
> See also: [add-credential](#command-juju-add-credential), [credentials](#command-juju-credentials), [remove-credential](#command-juju-remove-credential), [set-credential](#command-juju-set-credential)

**Aliases:** update-credentials

## Summary
Updates cloud credentials on this client or on a controller.

## Usage
```text
juju update-credential [options] [<cloud-name> [<credential-name>]]
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `-f`, `--file` |  | The YAML file containing credential details to update |
| `--force` | false | Request a forced controller update (controller superuser only) |
| `--region` |  | Cloud region to use when preparing the credential |

## Examples

Upload the cached material for one credential:

    juju update-credential aws production --controller mycontroller

Update the local AWS entry named production where AWS already has local credentials:

    juju update-credential aws production --file mine.yaml --client

Update the controller credentials for AWS from a file:

    juju update-credential aws --file mine.yaml --controller mycontroller

Update the Azure entry named production using the Brazil South region for preparation:

    juju update-credential azure production --region brazilsouth --file mine.yaml --controller mycontroller


## Details
Read replacement material from the client cache or a credentials YAML
file supplied with `--file`. Supply a cloud name when reading the
cache. A cloud name selects that cloud's entries. Supply a credential
name as well to select one entry. With a file, you can omit both names
to select entries across the file.

Use `--client` for this client, `--controller NAME` for the
signed-in account on that controller, or both flags for both stores. A
local update requires existing local credentials for the cloud. Use
`juju add-credential` to add the first local entry.

Use `--region` to select the region for credential preparation.
A controller update replaces the material used by all models bound to
that credential. Verify the cloud operation those models need. To
switch one model, use `juju set-credential`.

Only a controller superuser can force a controller update. After a
forced update, inspect the stored credential before relying on it.
Check the result for each credential before retrying.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/credential/