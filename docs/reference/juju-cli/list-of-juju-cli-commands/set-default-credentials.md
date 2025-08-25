(command-juju-set-default-credentials)=
# `juju set-default-credentials`
> See also: [credentials](#credentials), [add-credential](#add-credential), [remove-credential](#remove-credential), [autoload-credentials](#autoload-credentials)

**Aliases:** set-default-credentials

## Summary
Get, set, or unset the default credential for a cloud on this client.

## Usage
```juju default-credential [options] <cloud name> [<credential name>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--reset` | false | Reset default credential for the cloud |

## Examples

Set the default credential for the `google` cloud:

    juju default-credential google <credential>

View the default credential for the `google` cloud:

    juju default-credential google

Unset the default credential for the `google` cloud:

    juju default-credential google --reset


## Details

This command sets a locally stored credential to be used as a default.

Default credentials avoid the need to specify a particular set of
credentials when more than one credential is available on the client for a given cloud.