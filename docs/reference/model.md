---
myst:
  html_meta:
    description: "Juju model reference: logical workspaces for applications, resources, and configuration. The model record, model types, states, operations, watchers, and rules."
---

(model)=
# Model
```{audience} user
```

```{ibnote}
See also: {ref}`manage-models`
```

In Juju, a **model**  is an abstraction that holds {ref}`applications <application>` and application supporting components -- {ref}`machines <machine>`, {ref}`storage <storage>`, {ref}`network spaces <space>`, {ref}`relations <relation>`, etc.

A model is created by a {ref}`user <user>`, and owned in perpetuity by that user (or a new user with the same name), though it may also be used by any other user with model access level, within the limits of their level.

A model is created on a {ref}`controller <controller>`.  Both the model and the controller are associated with a {ref}`cloud <cloud>` (and a cloud {ref}`credential <credential>`), though they do not both have to be on the same cloud (this is a scenario where you have a 'multicloud controller' and where you may have 'cross-model relations (integrations)'). Any entities added to the model will use resources from that cloud.

One can deploy multiple applications to the same model. Thus, models allow the logical grouping of applications and infrastructure that work together to deliver a service or product.  Moreover, one can apply common {ref}`configurations <configuration>` to a whole model. As such, models allow the low-level storage, compute, network and software components to be reasoned about as a single entity as well.

(the-models-records)=
## The model's records

(the-model-record)=
### The model's identity

A model exists in two places, and the order of creation explains the
split. A model starts in the **controller** database: that is where
its authoritative life is kept -- the shared alive / dying / dead
cycle every entity has (see {ref}`Model states <the-model-states>`).

Creating a model also creates the model's own **model database**. The
model's identity is copied there as a read-only, denormalized summary
-- its UUID, its controller, its name and owner (the **qualifier**,
which disambiguates same-named models of different owners), its type,
its cloud, region and credential, and the controller-model flag --
one row per model database, enforced by the schema. The model
database's `model_life` table mirrors the controller-side life as a
best-effort facsimile. The model database also carries the model's
own records as separate tables: its {ref}`configuration
<model-configuration>`, {ref}`constraints <constraint>`,
{ref}`storage pools <storage>` and the target agent version.

The record's other stored discriminator is a role, not a type: the
controller-model flag marks the one model that runs Juju itself, and
either type can carry it -- a controller bootstrapped on a
Kubernetes cloud runs a CAAS controller model.

(the-controller-model)=
#### The controller model

**The controller model (`controller`).** This is your Juju management model. A Juju deployment will have just one controller model, which is created by default when you create a controller (`juju bootstrap`). It typically contains a single machine, for the controller (since Juju `3.0`, the `controller` application). If controller {ref}`high availability <high-availability>` is enabled, then the controller model would contain multiple instances. The `controller` model may also contain certain applications which it makes sense to deploy near the controller -- e.g., starting with Juju `3.0`, the `juju-dashboard` application.

(regular-model)=
#### Regular model

**Regular model.** This is your Juju workload model. A Juju deployment may have many different workload models, which you create manually (`juju add-model`). It is the model where you typically deploy your applications.

(the-model-in-the-data-model)=
### The model in the data model

The model's stored records are thin, and deliberately so: everything
the model *contains* lives in the entities' own tables (see
{ref}`the full spine <data-model-full-spine>`); the model row itself
is the denormalized identity summary, with four satellite records in
the model database -- the configuration key/value rows, the model's
constraints, the storage pools defined on the model, and the
agent-version singleton (the agent version the model's machines
should run and the latest one known). There is no life column on the
model row: life belongs to the controller database, and the model
database keeps only the facsimile (see
{ref}`Model states <the-model-states>`).

(the-model-states)=
### Model states

A model carries its life in the **controller** database -- the shared
alive / dying / dead cycle every entity has, plus an *activated* flag
that says the model is live on its controller (a model imported by a
{ref}`migration <the-model-migration>` starts unactivated and is
activated only once its agents have re-oriented).

#### Life

A model is created alive. The removal machinery marks it dying in the
controller database -- cascading to the model's applications, units,
relations and machines -- schedules the removal, and the
**Undertaker** worker (inside the controller agent) does the rest:
when the model reads dead it deletes the model's records and, as the
final act, the model's own Dqlite database (see
{ref}`Model removal <the-model-removal>`). The model database's life
facsimile is updated alongside -- it is what the model-side processes
read.

(types-of-model)=
(iaas-caas-models)=
### Types of model

Unlike the application or the unit, the model record does carry its
discriminator as a stored column. The model's **type** is derived
from the cloud at creation -- a Kubernetes cloud makes the model
`caas`; every other cloud makes it `iaas` -- and a model is one or
the other, never both. The type is more than a label: it selects the
machinery the model runs on, down to which {ref}`secret <secret>`
backend its secrets live in and whether its applications scale by
{ref}`pods <application>` or {ref}`machines <machine>`. The other
stored discriminator, the controller-model flag, is a role, not a
type (see {ref}`The model's identity <the-model-record>`).

(the-models-machinery)=
## The model's machinery

A model has machinery of its own: in the controller, the model's
workers run -- creation starts them, the Undertaker takes dying models
to dead, and migration moves a model between controllers.

(the-model-operations)=
### Model operations

Operations on models: creation, configuration, credential and
constraint updates, migration between controllers, and removal.

#### Model creation

A model is created with the create-model operation (for example,
`juju add-model`): the controller writes the model's records in both
databases, derives the model type from the cloud, and starts the
model's workers. Migration creates a model too -- an *importing*
model, unactivated until the migration's agents report success (see
{ref}`Model migration <the-model-migration>`).

(model-configuration)=
#### Model configuration

A model configuration is a rule or a set of rules that define the behavior of a model -- including the `controller` model. The keys and values are stored as rows on the model record and validated against the model config schema.

```{ibnote}
See more: {ref}`list-of-model-configuration-keys`,  {ref}`configure-a-model`
```

(the-model-migration)=
#### Model migration

```{audience} juju-dev
```

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

(the-model-removal)=
#### Model removal

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

(the-model-watchers)=
### Model watchers

```{audience} juju-dev
```

The model domain's watchable service exposes these watch surfaces --
what a watcher fires on, not who consumes it:

- **All models** -- the controller database's model table: the
  Undertaker's surface for models becoming dead.
- **Activated models** -- the models that have finished activating on
  the controller.
- **Model migration deletions** -- the Undertaker's second surface:
  models deleted by a migration's reap phase.
- **One model** -- changes to a single model's record.
- **One model's cloud credential** -- the credential a model tracks,
  so the model can react when the user changes it.

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-model-rules-and-errors)=
## Model rules and errors

The rules a **model identity** must satisfy:

- the model name is not empty, and the qualifier (its owner) must be
  a valid user name -- `admin`, or `alice@external` for a
  {ref}`user <user>` from an identity provider;
- the model type must be a known type (`iaas`, `caas`);
- model configuration keys must be known keys, and their values must
  pass the schema validation (`validation error` naming the invalid
  attributes).

The errors that encode them:

- *Existence and state*: `model not found`, `model already exists`,
  `model not activated`, `model already activated`,
  `model namespace not found`.
- *Validation*: `credential not valid`, `agent version not supported`,
  `agent stream not valid`, `constraints not found`.
- *Migration and backends*: `model not redirected`,
  `secret backend already set`, `user not found on model`.

(related-entities-model)=
## Entities related to the model

- **The controller** hosts models: every model record names its
  controller, and the model's authoritative life lives in the
  controller's database (see {ref}`controller <controller>`).
- A model contains **applications, machines, storage, spaces, subnets
  and relations** -- their records carry the model's UUID by
  construction, since the model database *is* the model
  (see {ref}`application <application>`, {ref}`machine <machine>`,
  {ref}`storage <storage>`, {ref}`space <space>`,
  {ref}`subnet <subnet>`, {ref}`relation <relation>`).
- **Clouds and credentials** ground the model: the model records the
  cloud, region and credential it deploys into (see
  {ref}`cloud <cloud>`, {ref}`credential <credential>`).
- **Users** hold access levels on the model (see {ref}`user <user>`).
- **Migration** moves the model between controllers (see
  {ref}`Model migration <the-model-migration>`).
- **Removal** owns the teardown, the Undertaker at its end (see
  {ref}`Model removal <the-model-removal>`).
- **The agent version** records what the model's machines should run
  (see {ref}`upgrading things <upgrading-things>`).
