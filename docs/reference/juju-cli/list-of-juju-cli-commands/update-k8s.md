(command-juju-update-k8s)=
# `juju update-k8s`
> See also: [add-k8s](#add-k8s), [remove-k8s](#remove-k8s)

## Summary
Updates an existing Kubernetes endpoint used by Juju.

## Usage
```juju update-k8s [options] <k8s name>```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `-f` |  | The path to a cloud definition file |

## Examples

    juju update-k8s microk8s
    juju update-k8s myk8s -f path/to/k8s.yaml
    juju update-k8s myk8s -f path/to/k8s.yaml --controller mycontroller
    juju update-k8s myk8s --controller mycontroller
    juju update-k8s myk8s --client --controller mycontroller
    juju update-k8s myk8s --client -f path/to/k8s.yaml


## Details

Update Kubernetes cloud information on this client and/or on a controller.

The Kubernetes cloud can be a built-in cloud such as MicroK8s.

A Kubernetes cloud can also be updated from a file. This requires a `<cloud name>` and
a `YAML` file containing the cloud details.

A Kubernetes cloud on the controller can also be updated just by using a name of a Kubernetes cloud
from this client.

Use `--controller` to update a Kubernetes cloud on a controller.

Use `--client` to update a Kubernetes cloud definition on this client.