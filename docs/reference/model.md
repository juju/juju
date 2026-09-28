---
myst:
  html_meta:
    description: "Juju model reference: logical workspaces for applications, resources, and configuration. Model declaration, persistence (record, states, types), execution (workers, migration, removal, watchers), and rules."
---

(model)=
# Model

In Juju, a **model** is an abstraction that holds {ref}`applications <application>` and application supporting components ({ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, and so on).

A model is created by a {ref}`user <user>`, and owned in perpetuity by that user (or a new user with the same name), though it may also be used by any other user with model access level, within the limits of their level.

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

(the-models-declaration)=
## Models in the declaration layer

You add, configure, or remove a model through one of Juju's clients;
adding a model requires {ref}`cloud add-model access
<user-access-cloud-add-model>`.

```{ibnote}
See also: {ref}`Juju | Manage models <manage-models>`, {ref}`Juju | Configure a model <configure-a-model>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(the-model-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - The model name is not empty, and the qualifier (its owner) must be a valid user name: `admin`, or `alice@external` for a {ref}`user <user>` from an identity provider.
  - Model configuration keys must be known keys, and their values must pass the schema validation (a `validation error` naming the invalid attributes).
- **Errors:**
  - **`model already exists`:** Triggered when creating a model with the same name and owner as an existing one. Remediation: choose another name or owner.
  - **`model credential not valid`:** Triggered when the cloud credential for the model does not validate. Remediation: check the credential against the model's cloud.
  - **`agent version not supported`:** Triggered when the model's target agent version is not supported. Remediation: target a supported agent version.
  - **`agent stream not valid`:** Triggered when the model's agent stream is not valid. Remediation: use a valid agent stream.

(the-models-persistence)=
(the-model-record)=
## Models in the persistence layer

A model has a record in Juju's databases, in two places. The
authoritative record lives in the controller database, identified by
its natural key, the model's name plus the {ref}`user <user>` that
qualifies it (the **qualifier**, which disambiguates same-named
models of different owners). The key is unique: a model with the same
name and owner cannot be added twice. The record carries the model's
type, its cloud, region and credential, and the controller-model flag
(see {ref}`Types of model <types-of-model>` for what each of those
discriminators does).

The model service in the controller performs the writes: creating a
model writes its records in both databases; configuring it rewrites
the configuration rows; migrating it creates an *importing* model,
unactivated until the migration's agents report success (see
{ref}`Model migration <the-model-migration>`).

The model's stored footprint is thin, and deliberately so: everything
the model *contains* lives in the entities' own tables (see
{ref}`the full spine <data-model-full-spine>`). The model's own
**model database** keeps a read-only, denormalized copy of its
identity (its UUID, its controller, its name and owner, its type,
its cloud, region and credential, and the controller-model flag), one
row per model database, enforced by the schema, and its `model_life`
table mirrors the controller-side life as a best-effort facsimile; it
is what the model-side processes read. The model database also carries
the model's own records as separate tables: its {ref}`configuration
<model-configuration>`, its {ref}`constraints <constraint>`, its
{ref}`storage pools <storage>` and the target agent version (the agent
version the model's machines should run and the latest one known; see
{ref}`upgrading things <upgrading-things>`). There is no life column
on the model row: life belongs to the controller database.

(the-model-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - A model is created alive. Its life is carried in the **controller**
    database, the shared alive / dying / dead cycle every entity has,
    plus an *activated* flag that says the model is live on its
    controller: a model imported by a {ref}`migration <the-model-migration>`
    starts unactivated and is activated only once its agents have
    re-oriented. The removal machinery in the execution layer is what
    moves a model to dying and dead; the record here stores it.
  - The natural key is unique: a model with the same name and owner
    cannot be added twice.
- **Errors:**
  - **`model not found`:** Triggered when querying a model by UUID or name that does not exist. Remediation: verify the model exists on the controller.
  - **`model not activated`:** Triggered when operating on an imported model before its agents have reported success. Remediation: wait for the migration's activation.
  - **`model already activated`:** Triggered when activating a model that is already live on its controller. Remediation: none; the model is already serving.
  - **`model namespace not found`:** Triggered when the model's Dqlite namespace record does not exist. Remediation: verify the model's database exists.
  - **`model constraints not found`:** Triggered when querying the model's constraints record that does not exist. Remediation: check that the model has constraints set.

(the-types-of-model)=
(types-of-model)=
(iaas-caas-models)=
### Types of model

Unlike the application or the unit, the model record does carry its
discriminator as a stored column. The model's **type** is derived
from the cloud at creation: a Kubernetes cloud makes the model
`caas`; every other cloud makes it `iaas`. A model is one or
the other, never both. The type is more than a label: it selects the
machinery the model runs on, down to which {ref}`secret <secret>`
backend its secrets live in and whether its applications scale by
{ref}`pods <application>` or {ref}`machines <machine>`. The other
stored discriminator, the controller-model flag, is a role, not a
type.

(the-controller-model)=
#### The controller model

**The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (since Juju `3.0`, the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller, for example starting with Juju `3.0`, the `juju-dashboard` application.

(regular-model)=
#### Regular model

**Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create explicitly. It is the model where you typically deploy your applications.

(the-models-execution)=
## Models in the execution layer

By the time the setting call returns, the record exists and the model's
workers are already running: a model is one of the entities with
machinery of its own. The machinery is actors and loops, not locations:
the controller agent runs one set of workers per model, each an actor
driving a loop over the model's state; nothing lives "in" the model.

- **The model's workers:** The controller agent's model worker manager
  runs one set per model; its compute provisioner loops over the
  model's unprovisioned machines, asking the cloud for instances and
  recording what comes back (see {ref}`machine provisioning
  <the-machines-machinery>`). The workers are drawn in the
  controller's worker tree (see {ref}`the agent <agent>`).
- **The Undertaker:** The controller-level actor whose loop is the
  dying-model work queue; a dying model is what it tears down (see
  below).
- **The migration master:** The source controller's actor for a
  migration; its loop is the migration's phase machine (see below).

(the-model-migration)=
### Model migration

```{ggarch}
:file: ../juju.ggarch
:sequence: Model migration
:alt: User calls juju migrate; the source controller's migration master quiesces the model's agents, sends the model envelope to the target controller's import, the agents validate against the target and re-orient, and the source controller activates the imported model and reaps the source.
:caption: Sequence diagram: Moving a model between controllers. The source controller's migration master worker drives the phase machine: it locks the model's agents down (quiesce), sends the model envelope (the YAML model export plus the controller-DB data) to the target, whose ordered import operations run with rollback; the agents validate against the target and rewrite their agent configuration to re-orient; the source then activates the imported model, transfers logs, and reaps the source model, redirecting active users.
```

A model can be moved between controllers with the migrate operation
(for example, `juju migrate <model> <target-controller>`). The
migration is a phase machine driven by the source controller: the
source's migration master worker quiesces the model's agents, sends
the model envelope (the YAML export of the model database plus the
model's controller-DB data) to the target controller, whose import
runs as ordered operations with rollback, then lets the agents
validate against the target and rewrite their agent configuration to
re-orient to it. Once the agents report success, the source
activates the imported model on the target, transfers the model's
logs, and reaps the source model (active users are redirected). An
aborted migration rolls the phases back on both sides.

```{important}

On Juju 4.0, migration targets a Juju 4.1 (or newer) controller: the
target must support the migration envelope facade version the source
sends. A 4.0 controller cannot yet be a migration target; the
prechecks reject the migration.

```

(the-model-removal)=
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
which tears the model down in dependency order (applications first,
then the machines, and the model's Dqlite database last, as the final
act after all cloud resources are released). The controller never
deletes its own database; the Undertaker does.

(the-model-execution-rules)=
### Execution rules and errors

- **Rules:**
  - The controller never deletes its own database: the model's Dqlite
    database goes last, after all cloud resources are released.
- **Errors:**
  - **`model not redirected`:** Triggered when looking up a model's completed redirect after a migration and none exists. Remediation: verify the migration finished its redirect phase.
  - **`secret backend already set`:** Triggered when setting the model's secret backend that already has one. Remediation: update the backend instead of setting it again.
  - **`user not found on model`:** Triggered when looking up a user's grant on a model and none exists. Remediation: grant the user access on the model.

(the-model-watchers)=
### Model watchers

The model domain's watchable service exposes these watch surfaces:

- **All models:** The controller database's model table: the
  Undertaker's surface for models becoming dead.
- **Activated models:** The models that have finished activating on
  the controller.
- **Model migration deletions:** The Undertaker's second surface:
  models deleted by a migration's reap phase.
- **One model:** Changes to a single model's record.
- **One model's cloud credential:** The credential a model tracks,
  so the model can react when the user changes it.

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).
