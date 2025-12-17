(command-juju-remove-cloud)=
# `juju remove-cloud`
> See also: [add-cloud](#add-cloud), [update-cloud](#update-cloud), [clouds](#clouds)

## Summary
Removes a cloud from Juju.

## Usage
```juju remove-cloud [options] <cloud name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--client` | false | Specifies whether to perform the operation on the local client. |
| `--target-controller` |  | Specifies the name of a JAAS managed controller to remove a cloud from. |

## Examples

    juju remove-cloud mycloud
    juju remove-cloud mycloud --client
    juju remove-cloud mycloud --controller mycontroller


## Details

Removes a cloud from Juju.

If `--controller` is used, also removes the cloud from the specified controller,
if it is not in use.

If `--client` is specified, Juju removes the cloud from this client.