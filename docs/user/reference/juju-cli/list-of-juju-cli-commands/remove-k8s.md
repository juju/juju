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
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |

## Examples

    juju remove-k8s myk8scloud
    juju remove-k8s myk8scloud --client
    juju remove-k8s --controller mycontroller myk8scloud


## Details

Removes the specified k8s cloud from this client.

If --controller is used, also removes the cloud 
from the specified controller (if it is not in use).

Use --client option to update your current client.