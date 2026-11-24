---
myst:
  html_meta:
    description: "Juju model reference: logical workspaces for applications, resources, and configuration. Learn controller and workload model types."
---

(model)=
# Model
```{ibnote}
See also: {ref}`manage-models`
```

In Juju, a **model**  is an abstraction that holds {ref}`applications <application>` and application supporting components -- {ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, etc.

A model is created by a {ref}`user <user>`, and owned in perpetuity by that user (or a new user with the same name), though it may also be used by any other user with model access level, within the limits of their level.

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** Each controller has one management model, created at {ref}`bootstrap <controller-bootstrap>` with the `controller` application. That application starts with one unit on a machine or in a Kubernetes pod and can grow to multiple {ref}`controller nodes <controller-nodes>`. The model can also host other applications, such as `juju-dashboard`.

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

(model-command-blocks)=
## Model command blocks

```{ibnote}
See also: {ref}`Juju CLI | Restrict commands on a model <restrict-commands-on-a-model>`
```

A **model command block** is a restriction that the controller enforces on operations that affect a model, regardless of which client sends the request. The block type determines which operations it can prevent:

- **Destroy block:** Prevents destruction of the model.
- **Remove block:** Prevents removal of objects such as machines, applications, units, and relations. It also prevents destruction of the model.
- **Change block:** Prevents operations that check for model changes, such as adding units. It also prevents object removals and model destruction.

If more than one block applies, the model destruction check looks for destroy, remove, and change blocks in that order. The object removal check looks for a remove block before a change block. These checks refuse the operation at the first block they find and return the message set on that block, if one was supplied.

Users with {ref}`read access to the model <user-access-model-read>` can list its active blocks. Users with {ref}`write access to the model <user-access-model-write>` can set or clear individual block types.

A user with {ref}`controller superuser access <user-access-controller-superuser>` can list models and their block types, or clear blocks across the controller. The controller-wide clearing operation accepts only an all-models request. If clearing fails, blocks already cleared in earlier models are not restored. For controller-specific checks, see {ref}`destroying the controller <destroying-a-controller>`.

## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```
