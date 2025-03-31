(command-juju-default-region)=
# `juju default-region`
> See also: [add-credential](#add-credential)

**Aliases:** set-default-region

## Summary
Sets the default region for a cloud.

## Usage
```juju default-region [options] <cloud name> [<region>]```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `--reset` | false | Reset default region for the cloud |

## Examples

    juju default-region azure-china chinaeast
    juju default-region azure-china
    juju default-region azure-china --reset


## Details
The default region is specified directly as an argument.

To unset previously set default region for a cloud, use --reset option.

To confirm what region is currently set to be default for a cloud, 
use the command without region argument.