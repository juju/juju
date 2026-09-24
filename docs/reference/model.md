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

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations (integrations)'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

(controller-model)=
## Model taxonomy

Models are of two types:

1. **The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (since Juju `3.0`, the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller -- e.g., starting with Juju `3.0`, the `juju-dashboard` application.

2. **Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

## Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```

(model-operations)=
## Model operations

(model-migration)=
### Model migration

```{ggarch}
:file: ../juju.ggarch
:sequence: Model migration
:alt: User calls juju migrate; the source controller's migration master quiesces the model's agents, sends the model envelope to the target controller's import, the agents validate against the target and re-orient, and the source controller activates the imported model and reaps the source.
:caption: Sequence diagram: Moving a model between controllers. The source controller's migration master worker drives the phase machine: it locks the model's agents down (quiesce), sends the model envelope -- the YAML model export plus the controller-DB data -- to the target, whose ordered import operations run with rollback; the agents validate against the target and rewrite their agent configuration to re-orient; the source then activates the imported model, transfers logs, and reaps the source model, redirecting active users.
```

A model can be moved between controllers with the migrate operation
(for example, `juju migrate <model> <target-controller>`). The
migration is a phase machine driven by the source controller: the
source's migration master worker quiesces the model's agents, sends
the model envelope -- the YAML export of the model database plus the
model's controller-DB data -- to the target controller, whose import
runs as ordered operations with rollback, then lets the agents
validate against the target and rewrite their agent configuration to
re-orient to it. Once the agents report success, the source
activates the imported model on the target, transfers the model's
logs, and reaps the source model (active users are redirected). An
aborted migration rolls the phases back on both sides.

```{important}

On Juju 4.0, migration targets a Juju 4.1 (or newer) controller: the
target must support the migration envelope facade version the source
sends. A 4.0 controller cannot yet be a migration target -- the
prechecks reject the migration.

```

## Model lifecycle

### Model removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Model removal
:alt: User calls juju destroy-model. Controller marks model Dying and fires watcher to Undertaker. Undertaker destroys all applications. Controller releases all machines and marks model Dead. Undertaker deletes model records and Dqlite database.
:caption: Sequence diagram: Model destruction, the largest removal, runs the same pattern at scale: the Undertaker worker inside the controller agent tears everything down in dependency order. The controller never deletes its own database -- the Undertaker does, as the final act after all cloud resources are released.
```


A model is destroyed with `juju destroy-model`. Destruction is the
largest removal and runs the same cooperative pattern at scale: the
controller marks the model Dying and wakes the Undertaker worker,
which tears the model down in dependency order -- applications first,
then the machines, and the model's Dqlite database last, as the final
act after all cloud resources are released. The controller never
deletes its own database -- the Undertaker does.

