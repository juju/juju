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

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

(model-access-levels)=
## Model access levels

On a particular model, {ref}`model read access <user-access-model-read>` permits inspecting model status; {ref}`model write access <user-access-model-write>` also permits workload changes such as adding units. A {ref}`model admin <user-access-model-admin>` can also share the model. A {ref}`controller superuser <user-access-controller-superuser>` can administer it without an explicit model grant.

The authenticated user who creates a model receives model admin. Only a controller superuser can choose another user's qualifier for the model name; that choice does not transfer the creator's grant.

(listing-users-with-model-access)=
### Listing users with model access

```{ibnote}
See also: {ref}`Juju CLI | List users of a model <list-users-of-a-model>`
```

The model user list reports enabled users with direct grants and their last recorded connections to that model. A model admin or controller superuser receives the roster. Other callers receive their own direct entry. An external user with {ref}`shared access on this model <shared-access-for-external-users>` can read it without appearing in the roster.

**Who can do this:** A signed-in user with at least {ref}`model read access <user-access-model-read>` or {ref}`controller superuser access <user-access-controller-superuser>`.

**Errors:** The controller can refuse this operation with these errors:

- Ends with `user not found on model`: The caller can read the model but has no direct entry to return. Ask a model admin to check the roster.

(changing-model-access)=
### Changing model access

```{ibnote}
See also: {ref}`Juju CLI | Change a user's model access <change-a-users-model-access>`, [Terraform Provider for Juju | Manage access to a model](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-models/#manage-access-to-a-model)
```

Changing the grant updates the user's direct access to this model. An external user can retain {ref}`shared access on the same model <shared-access-for-external-users>` after their direct grant is removed. Revocation is subject to the {ref}`model administrator requirement <keeping-an-active-model-administrator>`.

**Who can do this:** A {ref}`model admin <user-access-model-admin>` on this model or a {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** The controller can refuse this operation with these errors:

- Ends with `model would be left without an admin user`: The model administrator check failed. Grant another enabled user model `admin` on this model before retrying.

(keeping-an-active-model-administrator)=
### Keeping an active model administrator

Before removing or disabling a user, the controller checks that every model has another enabled user with an explicit {ref}`model admin grant <user-access-model-admin>`. Before revoking model access, the controller makes that check on the affected model. To count towards the check, a controller superuser must hold an explicit model admin grant.

## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller -- e.g., starting with Juju `3.0`, the `juju-dashboard` application.

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```
