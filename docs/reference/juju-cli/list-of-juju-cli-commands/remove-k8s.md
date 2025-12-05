(command-juju-remove-k8s)=
# `juju remove-k8s`
> See also: [add-k8s](#add-k8s)

## Summary
Removes a k8s cloud from Juju.

## Usage
```juju remove-k8s [options] <k8s name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Disables web browser for authentication. |
| `-c`, `--controller` |  | Performs the operation in the specified controller. |
| `--client` | false | Performs the operation on the local client. |

## Examples

    juju remove-k8s myk8scloud
    juju remove-k8s myk8scloud --client
    juju remove-k8s --controller mycontroller myk8scloud


## Details

Removes the specified Kubernetes cloud from this client.

If `--controller` is used, also removes the cloud
from the specified controller (if it is not in use).

The `--client` option can be used to update the current client.