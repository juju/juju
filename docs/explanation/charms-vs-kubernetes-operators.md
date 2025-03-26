(charms-vs-kubernetes-operators)=
# Charms vs. Kubernetes operators

A {ref}`charm <charm>` is an expansion and generalization of the [Kubernetes notion of an operator^](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

In the Kubernetes tradition, an operator is a container that drives lifecycle management, configuration, integration, and daily actions for an application. It handles instantiation, scaling, configuration, optimisation, networking, service mesh, observability, and Day 2 operations specific to that application. On the principle that an operator should ‘do one thing and do it well’, each operator drives a single application or service. However, it can be composed with other operators to deliver a complex application or service. Because operators package expert knowledge in a reusable and shareable form, they hugely simplify software management and operations.

In Juju, an operator does all that but supports even more uses and more infrastructures: With *charms* (coordinated by Juju) you can not only deploy an application but also connect it to other applications, and you can use not just Kubernetes clusters, but containers, virtual machines, and bare metal machines as well, on public or private cloud.
