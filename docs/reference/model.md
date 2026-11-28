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

A model has a qualified name. The user qualifier distinguishes models with the same name; it does not confer {ref}`model admin access <user-access-model-admin>`.

A model runs on a {ref}`cloud <cloud>` managed by its {ref}`controller <controller>`. That cloud may differ from the controller's own cloud. When a model uses cloud credentials, it draws its authentication material from a {ref}`controller credential <model-credentials>` for that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller -- e.g., starting with Juju `3.0`, the `juju-dashboard` application.

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```

(choosing-a-credential-for-a-new-model)=
## Choosing a credential for a new model

```{ibnote}
See also: {ref}`Juju CLI | Add a model <add-a-model>`, [Terraform Provider for Juju | Add a credential to a model](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#add-a-credential-to-a-model)
```

When a client omits the credential identifier, the controller looks for the signed-in user's sole {ref}`valid <credential-validity>`, non-revoked credential on the target cloud. With no unique match, the controller can create the model without a credential if the cloud supports empty authentication.

**Who can do this:** A user with at least {ref}`cloud add-model access <user-access-cloud-add-model>` on the target cloud or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Model creation can fail with these errors:

- `permission denied`. The caller lacks access to add a model on the target cloud. Ask for cloud add-model access.
- Ends with `a credential needs to be specified`. The cloud requires authentication and the controller found no unique credential. Supply a credential identifier for the target cloud.

(changing-a-models-credential)=
## Changing a model's credential

```{ibnote}
See also: {ref}`Juju CLI | Change the credential a model uses <change-the-credential-a-model-uses>`, [Terraform Provider for Juju | Add a credential to a model](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-credentials/#add-a-credential-to-a-model)
```

Changing a model's credential binds that model to another credential on the same cloud. {ref}`Updating a credential's material <updating-a-credential>` changes what all models bound to that credential use.

**Who can do this:** A {ref}`model admin <user-access-model-admin>` on this model or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** The controller can refuse a model credential change with these errors:

- `the operation has been blocked` or the configured block message. A change block is active. The controller checks the block before model access. Have the change block lifted before retrying.
- `permission denied`. The caller lacks access to make the switch. Ask a model admin or controller superuser.
- Contains `has different cloud to credential`. The replacement belongs to another cloud. Choose a credential for the model's cloud.
- Contains `not found` and `cloud credential`. The replacement is absent from the controller. Check its cloud, owner and name.
