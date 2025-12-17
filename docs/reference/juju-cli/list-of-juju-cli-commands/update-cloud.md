(command-juju-update-cloud)=
# `juju update-cloud`
> See also: [add-cloud](#add-cloud), [remove-cloud](#remove-cloud), [clouds](#clouds)

## Summary
Updates cloud information available to Juju.

## Usage
```juju update-cloud [options] <cloud name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Specifies whether to skip web browser for authentication. |
| `-c`, `--controller` |  | Specifies the controller to operate in. |
| `--client` | false | Specifies whether to perform the operation on the local client. |
| `-f` |  | Specifies the path to a cloud definition file. |

## Examples

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller
    juju update-cloud mymaas --client --controller mycontroller
    juju update-cloud mymaas --client -f path/to/maas.yaml


## Details

Updates cloud information on this client and/or on a controller.

A cloud can be updated from a file. This requires a `<cloud name>` and a `YAML` file
containing the cloud details.

This method can be used for cloud updates on the client side and on a controller.

A cloud on the controller can also be updated just by using a name of a cloud
from this client.

Use the `--controller` option to update a cloud on a controller.

Use `--client` to update cloud definition on this client.