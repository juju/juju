---
myst:
  html_meta:
    description: "Juju model reference: logical workspaces for applications, resources, and configuration. Learn controller and workload model types."
---

(model)=
# Model
```{ibnote}
See also:

- {ref}`Juju CLI | Manage models <manage-models>`
- {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
- {ref}`JAAS | Manage models <jaas:manage-models>`
```

In Juju, a **model**  is an abstraction that holds {ref}`applications <application>` and application supporting components -- {ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, etc.

A model has a qualifier and a name. The qualifier usually identifies the {ref}`user <user>` who created it, but it does not grant access. Other users can use the model within their own grants.

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** Each controller has one management model, created at {ref}`bootstrap <controller-bootstrap>` with the `controller` application. That application starts with one unit on a machine or in a Kubernetes pod and can grow to multiple {ref}`controller nodes <controller-nodes>`. The model can also host other applications, such as `juju-dashboard`.

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

(model-creation)=
## Model creation

```{ibnote}
See also:

- {ref}`Juju CLI | Add a model <add-a-model>`
- [Terraform Provider for Juju | Add a model](https://documentation.ubuntu.com/terraform-provider-juju/latest/howto/manage-models/#add-a-model)
- {ref}`JAAS | Create a model <jaas:create-a-model>`
```

A {ref}`controller superuser <user-access-controller-superuser>` can create a model, as can a user with {ref}`add-model access <user-access-cloud-add-model>` to the target cloud. A user who is not a superuser can create models only under their own qualifier.

If the request omits a cloud, the controller uses the controller model's cloud. It defaults the region from that model only for the same cloud. When no credential is supplied, the controller looks one up for the caller, not the user named by the qualifier. A model does not appear in listings until it has been activated.

An error after the controller creates the model record does not erase that record. An error after activation can leave the model visible even though setup failed. Check for the model before retrying with the same name and qualifier.

(model-listings-and-status)=
## Model listings and status

```{ibnote}
See also:

- {ref}`Juju CLI | List models on a controller <list-models-on-a-controller>`
- {ref}`Juju CLI | View model status <view-the-status-of-a-model>`
```

| Read | Result and access |
| --- | --- |
| All models | A {ref}`controller superuser <user-access-controller-superuser>` can list activated models on the controller, including the controller model. |
| Model summaries for a user | The user or a controller `superuser` can read summaries for models to which that user has access. A summary can report an error for one model while others succeed, including when access changes during the read. Only a controller superuser can request summaries for all models. |
| Model status | A controller `superuser` or that model's {ref}`admin <user-access-model-admin>` can read its machine and application counts. The result can report an error or a redirect per model. |
| Hosted model configuration | A controller `superuser` can read each workload model's configuration, cloud specification, and credential. The read excludes the controller model. A model's configuration or cloud read can fail in that model's result. |

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

(model-migration-contract)=
## Model migration

```{ibnote}
See also:

- {ref}`Juju CLI | Migrate a model <migrate-a-model>`
- {ref}`JAAS | Migrate a model to JAAS <jaas:migrate-a-model-to-jaas>`
- {ref}`JAAS | Migrate a model within JAAS <jaas:migrate-a-model-within-jaas>`
```

A {ref}`controller superuser <user-access-controller-superuser>` can request migration of a workload model to a different controller. A dry run checks the source, connects to the target and checks user compatibility unless that check was skipped. It does not run the target's import checks, such as checking for model name conflicts. A start request returns a migration ID or an error for each request entry.

During export, user API connections to the model can call model status, storage listing, ping, and a limited set of SSH discovery and key-related methods. Other calls return `model migration in progress`. During import, user API calls are refused with `migration in progress, model is importing`.

A migrated model can redirect user logins to the target controller. A local user must have had access to the model when it migrated to get the redirect. Agents are not redirected at login.

(model-agent-upgrades)=
## Model agent upgrades

```{ibnote}
See also:

- {ref}`Juju CLI | Upgrade a model <upgrade-a-model>`
- {ref}`JAAS | Upgrade a model <jaas:upgrade-a-model>`
```

A workload model has its own agent target version. The target's major, minor and patch version must not exceed the highest agent version recorded for the controller's nodes. A successful request changes the model's target, not the {ref}`controller's upgrade target <controller-upgrade>`, and does not mean the model's agents have finished upgrading.

## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```
