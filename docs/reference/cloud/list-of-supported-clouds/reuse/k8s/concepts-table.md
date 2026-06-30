If you are familiar with Kubernetes, the following maps Kubernetes concepts to their Juju equivalents:

| Kubernetes | Juju |
| - | - |
| [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) | {ref}`model <model>` |
| [node](https://kubernetes.io/docs/concepts/architecture/nodes/) | {ref}`machine <machine>` (on Kubernetes clouds, not managed by Juju) |
| [pod](https://kubernetes.io/docs/concepts/workloads/pods/) | {ref}`unit <unit>` |
| container | process in a unit |
| [service](https://kubernetes.io/docs/concepts/services-networking/service/) | {ref}`application <application>` |
