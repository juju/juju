(model)=
# Model
> See also:
> - {ref}`manage-models`
> - {ref}`application-modelling`


In Juju, a **model**  is an abstraction that holds {ref}`applications <application>` and application supporting components -- {ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, etc.

A model is created by a {ref}`user <user>`, and owned in perpetuity by that user (or a new user with the same name), though it may also be used by any other user with model access level, within the limits of their level. 

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud-substrate>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations (integrations)'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.


## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (since Juju `3.0`, the `controller` application). If [controller high availability](https://juju.is/docs/olm/high-availability-juju-controller) is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller -- e.g., starting with Juju `3.0`, the `juju-dashboard` application. 

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications. 


<!--
The picture below illustrates a typical deployment with one controller model containing one controller machine and a number of regular models, each containing multiple workload machines.

![machine](https://assets.ubuntu.com/v1/6d21bb7c-juju-models.png)
-->


<!--
A [model](https://juju.is/docs/models) is a canvas on a particular cloud/k8s-cluster. The model is used to group applications that are being operated together for a common purpose on a common substrate. The model will capture the applications, their integration, configuration, and resource allocation.

```{note}

Since each model is on a single substrate, and the service as a whole may span multiple clouds/k8s-clusters, it may require several models to provide the canvases for all the different applications in the service.

```

The [model](https://juju.is/docs/models) is a workspace for inter-related applications. It is an abstraction over applications, machines hosting them and other components such as persistent storage.

Inside the cluster, adding a Juju model creates a Kubernetes namespace with the same name. The namespace hosts all of the pods and other resources, except global resources.
-->


## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

> See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
