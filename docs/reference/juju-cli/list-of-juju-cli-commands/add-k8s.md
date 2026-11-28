(command-juju-add-k8s)=
# `juju add-k8s`
> See also: [remove-k8s](#command-juju-remove-k8s)

## Summary
Adds a Kubernetes endpoint and credential to Juju.

## Usage
```text
juju add-k8s [options] <k8s name>
```

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| `-B`, `--no-browser-login` | false | Do not use web browser for authentication |
| `-c`, `--controller` |  | Controller to operate in |
| `--client` | false | Client operation |
| `--cloud` |  | Specify the Kubernetes cluster cloud |
| `--cluster-name` |  | Specify the Kubernetes cluster to import |
| `--context-name` |  | Specify the Kubernetes context to import |
| `--credential` |  | Reserved for GKE discovery; unavailable for kubeconfig imports |
| `--region` |  | Specify the Kubernetes cluster region or cloud/region |
| `--skip-storage` | false | Skip cluster metadata and workload storage lookup |

## Examples

Register the staging context and its credential on this client:

    juju add-k8s staging-k8s --context-name staging --client

Register the current context and its credential on mycontroller:

    juju add-k8s staging-k8s --controller mycontroller

Register a Kubernetes cloud hosted in the AWS us-east-1 region:

    juju add-k8s staging-k8s --region aws/us-east-1 --client

Register a cluster named staging from another kubeconfig file:

    KUBECONFIG=staging-kubeconfig.yaml juju add-k8s staging-k8s --cluster-name=staging --client

Register the current context from kubeconfig data on standard input:

    kubectl config view --raw --minify --flatten | juju add-k8s staging-k8s --client


## Details

Creates a user-defined cloud based on a Kubernetes cluster.

The new Kubernetes cloud can then be used to bootstrap into, or it
can be added to an existing controller.

Specify a non default kubeconfig file location using `$KUBECONFIG` environment variable or pipe in file content from stdin.

The config file can contain definitions for different Kubernetes clusters,
use `--cluster-name` to pick which one to use.
It's also possible to select a context by name using `--context-name`.

When running `add-k8s` the underlying cloud/region hosting the cluster needs to be
detected to enable storage to be correctly configured. If the cloud/region cannot
be detected automatically, use either
  `--cloud <cloudType|cloudName> `to specify the host cloud
or
  `--region <cloudType|cloudName>/<someregion>` to specify the host
  cloud type and region.

Region is strictly necessary only when adding a Kubernetes cluster to a JAAS controller.
When using a standalone Juju controller, usually just `--cloud` is required.

Once Juju is aware of the underlying cloud type, it looks for a suitably
configured storage class to provide workload storage. A cluster where none is
found is still added, with a warning: the storage class is nominated at
bootstrap time, by scheduling a Kubernetes storage pool and setting the
`storage-default-filesystem-source` model configuration to that pool.

The kubeconfig context must let Juju prepare an administrative
credential in the cluster. The new credential has the same name as the
cloud. Use `--client` for the client, `--controller NAME` for the
controller, or both. Specify a destination when piping kubeconfig data.

For a cluster without workload storage, use `--skip-storage` to skip
the cluster metadata and storage lookup. Juju still prepares the credential.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/cloud/