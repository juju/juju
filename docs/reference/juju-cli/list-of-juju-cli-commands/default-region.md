(command-juju-default-region)=
# `juju default-region`
> See also: [add-credential](#add-credential)

**Aliases:** set-default-region

## Summary
Gets, sets, or unsets the default region for a cloud on this client.

## Usage
```juju default-region [options] <cloud name> [<region>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--reset` | false | Reset default region for the cloud |

## Examples

Set the default region for the `azure-china` cloud to `chinaeast`:

    juju default-region azure-china chinaeast

Get the default region for the `azure-china` cloud:

    juju default-region azure-china

Unset the default region for the `azure-china` cloud:

    juju default-region azure-china --reset