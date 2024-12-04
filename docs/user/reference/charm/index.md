(charm)=
# Charm

```{toctree}
:hidden:

channel
revision
charm-taxonomy
charm-development-best-practices
charm-naming-guidelines
charm-maturity
talking-to-a-workload-control-flow-from-a-to-z
charm-environment-variables
```


In Juju, a **charm** is an operator -- software that wraps an [application](https://juju.is/docs/juju/application)  and that contains all of the instructions necessary for deploying, configuring, scaling, integrating, etc., the application 
on any [cloud](https://juju.is/docs/juju/cloud) using [Juju](https://juju.is/docs/juju). 
 
<!--
- business logic encapsulated in reusable software packages that automate every aspect of an [application](https://juju.is/docs/juju/application)'s life.
-->

> See more: [SDK | List of files in the charm project](https://juju.is/docs/sdk/list-of-files-in-the-charm-project), [SDK | The Juju execution flow for a charm](https://juju.is/docs/sdk/the-juju-execution-flow-for-a-charm)

Charms are publicly available on {ref}`charmhub`.

Charms are currently of two kinds, depending on the target deployment substrate:

1. **Machine charms:** Charms made to deploy on a bare-metal server, virtual machine, or system container.
    - [Charmhub | Machine charms](https://charmhub.io/?base=vm&type=all)
1. **Kubernetes charms:** Charms built to deploy on Kubernetes.
    - [Charmhub | Kubernetes charms](https://charmhub.io/?base=kubernetes&type=all)
         - [Charmhub | example Kubernetes charm: PostgreSQL K8s](https://charmhub.io/postgresql-k8s)

Charms are currently developed using {ref}`charmcraft` and {ref}`ops`.

> See more: {ref}`charm-taxonomy`, {ref}`charms-vs-kubernetes-operators` 

While the quality of individual charms may vary, charms are intended to implement a general and comprehensive approach for operating applications.

> See more: {ref}`charm-maturity`

<!--WORK THIS INTO THE MAIN TEXT

A charm is an operator – business logic encapsulated in a reusable software package that automates every aspect of an application’s life.

Charms written with ops support Kubernetes using Juju’s “sidecar charm” pattern, as well as charms that deploy to Linux-based machines and containers.

Charms should do one thing and do it well. Each charm drives a single application and can be integrated with other charms to deliver a complex system. A charm handles creating the application in addition to scaling, configuration, optimisation, networking, service mesh, observability, and other day-2 operations specific to the application.

The ops library is part of the Charm SDK (the other part being Charmcraft). Full developer documentation for the Charm SDK is available at https://juju.is/docs/sdk.

To learn more about Juju, visit https://juju.is/docs/olm.

-->




<!--
The simplest scenario is when a charm is deployed (by the Juju client) with the `deploy` command without any options to qualify the request. By default, a new instance will be created in the backing cloud and the application will be installed within it:

![machine](https://assets.ubuntu.com/v1/411232ff-juju-charms.png)

-->

<!-- TODO clarify charm vs. application. Clarify general understanding of "application" vs. Juju notion.

A charm has a one-to-one correspondence to an application. 

To deploy a charm with `juju` is to deploy an application. >> maybe not clear in this form

Every time you deploy a charm with Juju, those charms turn up as applications in the Juju model. But will an end user think of those things as applications or just pieces of the overall model. E.g., core database charm + 2 charm with supporting infrastructure. Is there a hierarchy of applications in the user's mind? Maybe yes. A user might think of just the main component as "the application".

Puppet and Chef don't really do Day 2. Juju does that. Also, relations. Helps you reason about the whole solution.

The nice thing about Juju relations is that they help you mix and match. Every application has an endpoint. Your solution becomes a bag of components that you can combine. So, with Juju you can scale up and down, but also substitute other components quite easily as well. 

-->

<!--TODO introduce subordinate charms-->

<!--
controller = conductor
machine agents = musicians
charms = musical instruments
The Juju client is a member of the audience that hands the conductor the sheet music. It tells the conductor: Here's the music I want you to play.

It is manipulated by a machine agent to

It's the fundamental building block 

represent the distilled knowledge of experts

-->
