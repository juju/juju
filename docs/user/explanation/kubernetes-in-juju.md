(kubernetes-in-juju)=
# Kubernetes in Juju

Juju supports both traditional machine clouds as well as Kubernetes clouds. If you are familiar with Kubernetes, there's a mapping between Kubernetes and Juju concepts:

| Kubernetes | Juju |
|-|-|
| [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) | {ref}`model <model>` |
|  [node](https://kubernetes.io/docs/concepts/architecture/nodes/) | {ref}`machine <machine>`; Juju does not manage this for Kubernetes |
| [pod](https://kubernetes.io/docs/concepts/workloads/pods/) | {ref}`unit <unit>`  |
| container | process in a unit |
| [service](https://kubernetes.io/docs/concepts/services-networking/service/) | {ref}`application <application>` |


The rest of this document expands on this mapping.

## Namespace and Model

Both namespaces and models allow for the aggregation of a set of resources into a common "context". However, a Juju model must be part of a Juju cloud, meaning it inherently links the managed applications to an underlying cloud infrastructure, whereas a Kubernetes namespace is not part of any higher grouping; merely a logical partition within a cluster—which may be cloud-hosted.

## Node and Machine

While both **nodes** and **machines** refer to physical or virtual resources that run workloads, in a Juju deployment on Kubernetes, Juju does not represent nodes as machines.
Instead, Juju delegates the work of handling nodes to the Kubernetes cluster,
and only manages **pods** directly.

## Pod and Unit

**Pods** and **units** represent deployable instances that
deploy code into a container or process. However, in Juju, one unit is designated as the leader, which will be the unit handling the lifecycle of the application. Pods lack this functionality, meaning you would need to manually implement leader election to enable this type of pod architecture in Kubernetes.

## Service and Application

Both similar in concept, **services** and **applications** allow the integration of other services/applications within the cluster and can also
be exposed to enable access from the external world to the cluster.
A key difference is that applications can be automatically integrated with other applications through defined relations, provided that the applications' {ref}`endpoints <application-endpoint>` are
compatible with each other. On the other hand, the integration between services must be done
manually, using the services' IP addresses or DNS names as their integration points.
