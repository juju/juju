---
myst:
  html_meta:
    description: "Juju watcher reference: what a watcher is, the kinds the controller API serves and what each one carries, how a holder drives a watcher with Next and Stop, the change stream that backs every watcher, and who holds one."
---

(watcher)=
# Watcher

A **watcher** is an object of the {ref}`controller <controller>` API (the set of calls that {ref}`clients <client>` and {ref}`agents <agent>` make to the controller) that tells its holder when something it follows has changed. A watch call of a facade (a named group of API calls) creates a watcher and returns its ID. The holder receives each change with `Next` and releases the watcher with `Stop`.

(watcher-kinds)=
## Watcher kinds

The watcher kinds are the shapes a delivery can take, from the empty signal to the typed payloads of one part of the model:

- **Notify:** an empty signal with no data. The first signal says that the watch is active. Each later signal says that the watched thing changed.
- **Strings:** the keys of the records that changed. The first list holds the current set of keys. Each later list holds the keys that were added, changed, or removed. An entity watcher is a strings watcher whose keys are the tags of entities.
- **Relation units:** the units in the scope of one relation, with the version of each unit's settings, and the units that departed. The first delivery holds the current membership of the relation.
- **Relation status:** the life and suspended status of a relation. The first delivery holds the current values.
- **Offer status:** the status of an offer. The first delivery holds the current values.
- **Migration status:** the phase of a {ref}`model migration <model-migration>`, with the API addresses and CA certificate of the source and target controllers. Each delivery holds the current details of the migration.
- **Machine storage IDs:** pairs of a machine tag and the tag of a volume or filesystem attachment that changed.
- **Secret trigger:** the URI of a secret, with a revision and the time of its next trigger.
- **Secret revision:** the URI of a secret, with a revision that changed.
- **Secret backend rotation:** the ID and name of a secret backend, with the time of its next token rotation.
- **Model summaries:** the whole current state of a {ref}`model <model>` on the first delivery, then the summaries that changed.
- **Remote relation:** relation units deliveries expanded with the settings values, for use across a model boundary.

The change-stream sections of the entity pages name the watch calls that hand out the watchers of their domain.

(watcher-lifecycle)=
## Watcher lifecycle

The watcher lifecycle runs from the watch call that creates the watcher to the `Stop` that releases it:

- **Creation:** a watch call of a facade (a named group of API calls) asks the facade's service for a watcher, registers the watcher with the caller's connection, and returns its ID. The first delivery holds the current state of the watched thing; for a notify watcher it is one signal, which says that the watch is active. It comes in the answer of the watch call, except for the summary and migration-status kinds, which deliver it on the first `Next`.
- **`Next`:** returns when the watched thing has changed since the most recent `Next`, or since the watch call when it is the first, and delivers the changes.
- **`Stop`:** releases the watcher on the controller. Stopping a watcher that is already stopped returns no error. The client libraries send `Stop` on the holder's behalf when the holder stops using the watcher.

The controller also removes every watcher of a connection when the connection closes. A holder that meets an error is told by one of these messages, each with its code:

- **`watcher has been stopped`** (code `stopped`): a `Next` that was waiting when the watcher was stopped. *Remediation:* None needed; the holder has released the watcher.
- **`watcher "<id>" not found`** (code `not found`): a call on the ID of a released watcher. *Remediation:* Ask the facade for a new watcher.
- **`unknown watcher id`** (code `not found`): a call on an ID that belongs to a watcher of another kind. *Remediation:* Call the watcher through the facade of its own kind.

(watcher-backing)=
## The change stream

The change stream is the flow of change records from the {ref}`controller and model databases <database>` to the watchers. Each write that a watcher can follow adds a change record to the database through a database trigger.

A worker of the {ref}`controller <controller>` polls the change log. It groups the changes that one poll of the log finds. The changes of one record within a group are coalesced into the last of them. Each group is forwarded to the watcher subscriptions whose filter matches.

A watcher holds one subscription. The filter names a namespace (the part of the database the changes come from) and the change types to follow: create, update, and delete.

A change record holds the change type, the namespace, and the key of the changed record, with no data. By the time a delivery arrives, what changed may already have changed again, so the holder reads the current state from the controller.

(watcher-holders)=
## Who holds a watcher

The holders of a watcher are the {ref}`clients <client>` and {ref}`agents <agent>` that receive one through a watch call. A client acts for its user. An agent acts for its own machine or unit.

A facade hands a watcher to its caller through each of its watch calls: the facade asks its service for a watcher, registers the watcher with the caller's connection, and returns the ID. Two examples:

- **The storage provisioner:** the `StorageProvisioner` facade hands a machine agent a watcher for each entity it names, so the agent hears the volumes, filesystems, and block devices that it must provision.
- **The migration minion:** the `MigrationMinion` facade hands a machine or unit agent a watcher of the model's migration, so the agent hears each phase of the migration as it happens.

Inside an agent, a {ref}`worker <agent-workers>` holds the watcher and runs its handler for each delivery. On a client, the library turns the watcher into a local stream of changes that the client's code reads.
