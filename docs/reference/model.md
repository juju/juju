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
(the-model-declaration-rules)=
## Models in the declaration layer

You add, configure, or remove a model through one of Juju's clients.

- **Adding:** Adding a model requires {ref}`cloud add-model access <user-access-cloud-add-model>`.
  - *Rule:* The model name is not empty, and the qualifier (its owner) must be a valid user name: `admin`, or `alice@external` for a {ref}`user <user>` from an identity provider.
  - *Related errors:*
    - **`model already exists`**: *Trigger:* Creating a model with the same name and owner as an existing one. *Remediation:* Choose another name or owner.
    - **`model credential not valid`**: *Trigger:* The cloud credential for the model does not validate. *Remediation:* Check the credential against the model's cloud.
    - **`agent version not supported`**: *Trigger:* The model's target agent version is not supported. *Remediation:* Target a supported agent version.
    - **`agent stream not valid`**: *Trigger:* The model's agent stream is not valid. *Remediation:* Use a valid agent stream.
- **Configuring:** Model configuration is set through a client.
  - *Rule:* Model configuration keys must be known keys, and their values must pass the schema validation (a `validation error` naming the invalid attributes).
- **Removing:** A model is destroyed through a client (see {ref}`model removal <the-model-removal>`).

```{ibnote}
See also: {ref}`Juju | Manage models <manage-models>`, {ref}`Juju | Configure a model <configure-a-model>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(the-models-persistence)=
(the-model-record)=
(the-model-persistence-rules)=
## Models in the persistence layer

A model has a record in Juju's databases, in two places.

- **Controller record:** The authoritative record lives in the controller database, identified by its natural key, the model's name plus the {ref}`user <user>` that qualifies it (the **qualifier**, which disambiguates same-named models of different owners). The record carries the model's type, its cloud, region and credential, and the controller-model flag (see {ref}`Types of model <types-of-model>` for what each of those discriminators does).
  - *Rule:* The natural key is unique: a model with the same name and owner cannot be added twice.
  - *Related errors:*
    - **`model not found`**: *Trigger:* Querying a model by UUID or name that does not exist. *Remediation:* Verify the model exists on the controller.
    - **`secret backend already set`**: *Trigger:* Setting a secret backend on a model record that already has one. *Remediation:* Update the backend instead of setting it again.
    - **`user not found on model`**: *Trigger:* Looking up a user's grant on a model when none exists. *Remediation:* Grant the user access on the model.
- **Life and activation:** The model is created alive. Its life is carried in the **controller** database, the shared alive / dying / dead cycle every entity has, plus an *activated* flag that says the model is live on its controller: a model imported by a {ref}`migration <the-model-migration>` starts unactivated and is activated only once its agents have re-oriented. The removal machinery in the execution layer is what moves a model to dying and dead; the record here stores it. There is no life on the model's own record in the model database: life belongs to the controller database.
  - *Related errors:*
    - **`model not activated`**: *Trigger:* Operating on an imported model before its agents have reported success. *Remediation:* Wait for the migration's activation.
    - **`model already activated`**: *Trigger:* Activating a model that is already live on its controller. *Remediation:* None; the model is already serving.
- **Namespace record:** Names the model's own Dqlite database.
  - *Related error:*
    - **`model namespace not found`**: *Trigger:* The model's Dqlite namespace record does not exist. *Remediation:* Verify the model's database exists.
- **Model database copy:** The model's stored footprint is thin, and deliberately so: everything the model *contains* lives in the entities' own records (see {ref}`the database <database>`). The model's own **model database** keeps a read-only, denormalized copy of its identity (its UUID, its controller, its name and owner, its type, its cloud, region and credential, and the controller-model flag), one record per model database, enforced by the schema, and its model-life record mirrors the controller-side life as a best-effort facsimile; it is what the model-side processes read.
- **Model's own records:** The model database also carries the model's own records: its {ref}`configuration <model-configuration>`, its {ref}`constraints <constraint>`, its {ref}`storage pools <storage>` and the target agent version (the agent version the model's machines should run and the latest one known; see {ref}`upgrading your deployment <upgrade-your-deployment>`).
  - *Related error:*
    - **`model constraints not found`**: *Trigger:* Querying the model's constraints record when it does not exist. *Remediation:* Check that the model has constraints set.

**Writers:** The model service in the controller performs the writes: creating a model writes its records in both databases; configuring it rewrites the configuration records; migrating it creates an *importing* model, unactivated until the migration's agents report success (see {ref}`Model migration <the-model-migration>`).

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

**The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller, for example the `juju-dashboard` application.

(regular-model)=
#### Regular model

**Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create explicitly. It is the model where you typically deploy your applications.

(the-models-execution)=
(the-model-execution-rules)=
## Models in the execution layer

By the time the setting call returns, the record exists and the model's
workers are already running: a model is one of the entities with
machinery of its own. The machinery is actors and loops, not locations:
the controller agent runs one set of workers per model, each an actor
driving a loop over the model's state; nothing lives "in" the model.

- **The model's workers:** The controller agent's model worker manager runs one set per model; its compute provisioner loops over the model's unprovisioned machines, asking the cloud for instances and recording what comes back (see {ref}`machine provisioning <the-machines-machinery>`). The workers are drawn in the controller's worker tree (see {ref}`the agent <agent>`).
- **The Undertaker:** The controller-level actor whose loop is the dying-model work queue; a dying model is what it tears down (see below).
- **The migration master:** The source controller's actor for a migration; its loop is the migration's phase machine (see below).

(the-model-migration)=
### Model migration

A model can be moved between controllers with the migrate operation
(for example, `juju migrate <model> <target-controller>`). The
migration is a phase machine driven by the source controller: the
source's migration master worker quiesces the model's agents (locks them down), sends
the model envelope (the YAML export of the model database plus the
model's controller-DB data) to the target controller, whose import
runs as ordered operations with rollback, then lets the agents
validate against the target and rewrite their agent configuration to
re-orient to it. Once the agents report success, the source
activates the imported model on the target, transfers the model's
logs, and reaps the source model (active users are redirected). An
aborted migration rolls the phases back on both sides.

- *Rule:* The target must be a Juju 4.1 (or newer) controller: it must support the migration envelope facade version the source sends (version 8 of the migration target facade). A 4.0 controller cannot be a migration target.
- *Related errors:*
  - **`target controller does not support the model migration format; upgrade the target controller to Juju 4.1 or later`**: *Trigger:* The prechecks find a target whose migration target facade is older than the envelope needs. *Remediation:* Upgrade the target controller to Juju 4.1 or later.
  - **`model not redirected`**: *Trigger:* Looking up a model's completed redirect after a migration when none exists. *Remediation:* Verify the migration finished its redirect phase.

(the-model-removal)=
### Model removal

A model is destroyed with `juju destroy-model`. Destruction is the
largest removal and runs the same cooperative pattern at scale: the
controller marks the model Dying and wakes the Undertaker worker,
which tears the model down in dependency order (applications first,
then the machines, and the model's Dqlite database last, as the final
act after all cloud resources are released). The controller never
deletes its own database; the Undertaker does.

- *Rule:* The controller never deletes its own database: the model's Dqlite database goes last, after all cloud resources are released.

(the-model-watchers)=
### Model watchers

The model domain's watchable service exposes these watch surfaces:

- **All models:** The controller database's model records: the
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
