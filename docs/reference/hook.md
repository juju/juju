---
myst:
  html_meta:
    description: "Juju hooks reference: charm lifecycle events, hook execution, idempotency, and Ops framework integration for application state management."
---

(hook)=
# Hook

In Juju, a **hook** is a notification from  the controller agent through the unit agent to the charm that the internal representation of Juju has changed in a way that requires a reaction from the charm so that the unit's state and the controller's state can be reconciled.


For a charm written with [Ops](https://ops.readthedocs.io/en/latest/), Juju hooks are translated into Ops events, specifically, into classes that inherit from [`HookEvent`](https://ops.readthedocs.io/en/latest/reference/ops.html#ops.HookEvent).

Whenever a hook event is received, the associated event handler should ensure the current charm configuration is properly reflected in the underlying application configuration.
Invocations of associated handlers should be idempotent and should not make changes to the environment, or restart services, unless there is a material change to the charm's configuration, such as a change in the port exposed by the charm, addition or removal of a relation which may require a database migration or a "scale out" event for high availability, or similar.

Handlers must not assume that the underlying applications or services have been started and should not assume anything about order of hook execution, except in limited situations described below.

## Hook kinds

All hooks share a lot of common behaviour in terms of the environment in which they run, how charms
are notified that a hook event has occurred, how errors are reported, and how a user might respond to
a unit being in an error state due to a failed hook execution etc.

Some hooks can also be grouped according to the Juju subsystem they represent (relations, secrets, storage, workload/Pebble).

```{ibnote}
See more: {ref}`list-of-hooks`
```

(hook-execution)=
## Hook execution

Hooks are run with environment variables set by Juju to expose relevant contextual configuration to the charm.

The Juju environment variables are set in addition to those supplied by the execution environment itself.

All hooks get a common set of environment variables; in addition, some hooks (or hook kinds) also get hook (hook kind) specific environment variables, as specified in the documentation for each hook.

```{ibnote}
See more: {ref}`list-of-hooks`
```

## Hook ordering

A charm's lifecycle consists of distinct **phases**:
* installation
* operation
* upgrade
* teardown

Generally, no assumptions can be made about the order of hook execution. However, there are some limited guarantees
about hook sequencing during install, upgrade, and relation removal.

In normal operation, a unit will run at least the {ref}`hook-install`, {ref}`hook-start`, {ref}`hook-config-changed` and {ref}`hook-stop` hooks over the course of its lifetime.

(installation-phase)=
### Installation phase

When a charm is first deployed, the following hooks are executed in order before a charm reaches its operation phase:

* {ref}`hook-storage-attached` (for machine charms if the unit has storage)
* {ref}`hook-install`
* {ref}`hook-config-changed`
* {ref}`hook-start`

```{note}
Only machine charms have the behaviour where the {ref}`hook-storage-attached` hook must run before the {ref}`hook-install` hook.
See {ref}`storage hooks <storage-hooks>` for more details.
```

(operation-phase)=
### Operation phase

This phase occurs when a charm has completed its installation operations and starts responding to events which correspond to interesting (relevant) changes to the Juju model.
The behaviour when in this phase can be explained by understanding the events which trigger the execution of the different hook kinds, and the context in which the hooks execute.
This is covered in subsequent sections where hook kinds are explored in more detail.

(upgrade-phase)=
### Upgrade phase

When a charm is upgraded, the {ref}`hook-upgrade-charm` hook is followed by a {ref}`hook-config-changed` hook.

The {ref}`hook-upgrade-charm` hook always runs once immediately after the charm directory
contents have been changed by an unforced charm upgrade operation, and *may* do
so after a forced upgrade; but will *not* be run after a forced upgrade from an
existing error state. (Consequently, neither will the {ref}`hook-config-changed` hook that
would ordinarily follow the {ref}`hook-upgrade-charm` hook.)

(teardown-phase)=
### Teardown phase

When a unit is to be removed, the following hooks are executed:

* {ref}`hook-stop`
* {ref}`hook-storage-detaching` | {ref}`hook-relation-broken` (in any order)
* {ref}`hook-remove`

The {ref}`hook-remove` hook is the last hook a unit will ever see before being removed.

## Errors in hooks

Hooks should ideally be idempotent, so that they can fail and be re-executed
from scratch without trouble. Charm code in hooks does not have complete control
over the times the hook might be unexpectedly aborted: if the unit agent process is
killed for any reason while running a hook, then when it recovers it will treat that
hook as having failed -- just as if it had returned a non-zero exit code -- and
request user intervention.

Hooks should be written to expect that users could (and probably will) attempt to
re-execute failed hooks before attempting to investigate or understand the situation.
Hook code should therefore make every effort to ensure hooks are idempotent when
aborted and restarted.

The most sophisticated charms will consider the nature of their operations with
care, and will be prepared to internally retry any operations they suspect of
having failed transiently (to ensure that they only request user intervention in
the most trying circumstances) as well as careful to log any relevant
information or advice before signalling the error.

(list-of-hooks)=
## List of hooks
<!-- > [Source](https://github.com/juju/juju/blob/main/internal/charm/hooks/hooks.go) -->

This section gives the complete list of hooks.

Where hooks belong to a kind, we nest them under that kind; otherwise, under "other". Thus:

* {ref}`relation-hooks`
* {ref}`secret-hooks`
* {ref}`storage-hooks`
* {ref}`workload-hooks`
* {ref}`other-hooks`

In all cases we cover
- What triggers the hook?
- Which environment variables is it executed with?
- Who gets it?

(relation-hooks)=
### Relation hooks

Relation hooks are used to inform a charm about changes to related applications and units.

When an application becomes involved in a relation, each one of its units will start to receive relevant hook events
for that relation. From the moment the relation is created, any unit involved in it can interact with it.
In practice, that means using one of the following hook commands that the Juju unit agent exposes to the charm:
- `relation-ids`
- `relation-list`
- `relation-get`
- `relation-set`

For every endpoint defined by a charm, relation hook events are named after the charm endpoint:

* `<endpoint>-relation-created`
* `<endpoint>-relation-joined`
* `<endpoint>-relation-changed`
* `<endpoint>-relation-departed`
* `<endpoint>-relation-broken`

For each charm endpoint, any or all of the above relation hooks can be implemented.
Relation hooks operate in an environment with additional environment variables available:

* `JUJU_RELATION` holds the name of the relation. This is of limited value, because every relation hook already "knows" what charm relation it was written for; that is, in the `foo-relation-joined` hook, `JUJU_RELATION` is `foo`.
* `JUJU_RELATION_ID` holds the ID of the relation. It is more useful, because it serves as unique identifier for a particular relation, and thereby allows the charm to handle distinct relations over a single endpoint. In hooks for the `foo` charm relation, `JUJU_RELATION_ID` always has the form `foo:<id>`, where ID uniquely but opaquely identifies the runtime relation currently in play.
* `JUJU_REMOTE_APP` holds the name of the related application.

Furthermore, all relation hooks except `relation-created` and `relation-broken` are notifications about some specific unit of a related application, and operate in an environment with the following additional environment variables available:

* `JUJU_REMOTE_UNIT` holds the name of the current related unit.

For every relation in which a unit participates, hooks for the appropriate charm relation are run according to the following rules.

The `relation-created` hook always runs once when the relation is first created, before any related units are processed.

The `relation-joined` hook always runs once when a related unit is first seen.
As related applications are scaled up, each unit will receive `<endpoint>-relation-joined`, once for each related unit being added.

The `relation-changed` hook for a given unit always runs once immediately following the `relation-joined` hook for that unit,
and subsequently whenever any related unit for which this unit has read access changes its settings (by calling the `relation-set` hook command and exiting without error).

```{note}
"Immediately" only applies within the context of this particular relation --that is, when
`foo-relation-joined` is run for unit `bar/99` in relation ID `foo:123`, the only guarantee is that
the next hook to be run in relation ID `foo:123` will be `foo-relation-changed` for `bar/99`.
Non-relation hooks may intervene, as may hooks for other relations, and even for other `foo` relations.
```

A unit will depart a relation when either the relation or the unit itself is marked for termination. When this happens,
for every known related unit (those which have joined and not yet departed), the `relation-departed` hook is run.
After the `relation-departed` hook has run for a given unit, no further notifications will be received from that unit;
however, its settings will remain accessible via the `relation-get` hook command for the complete lifetime of the relation.
It's also still possible to call the `relation-set` hook command.

The `relation-departed` hook also sets an additional environment variable:

* `JUJU_DEPARTING_UNIT` holds the name of the related unit departing the relation.

The `relation-broken` hook is not specific to any unit, and always runs once when the local unit is ready to depart the relation itself.
Before this hook is run, a `relation-departed` hook will be executed for every unit known to be related; it will never run while the relation
appears to have members, but it may be the first and only hook to run for a given relation.
The `stop` hook will not run until all relations have run the `relation-broken` hook.

```{note}
So what's the difference between `relation-departed` and `relation-broken`?
Think of `relation-departed` as the "saying goodbye" event; relation settings can still be read and a relation can even still be set.
Once `relation-broken` fires, however, the relation no longer exists. This is a good spot to do any final cleanup, if necessary.
Both `relation-departed` and `relation-broken` will always fire, regardless of how the relation is terminated.
```

(hook-relation-broken)=
#### `<endpoint>-relation-broken`

*What triggers it?*

- a non-peer relation being removed;
- a unit participating in a non-peer relation being removed, even if the relation is otherwise still alive (through other units); or
- an application involved in a non-peer relation being removed.

This hook is fired only once per unit per relation and is the exact inverse of `relation-created`. `relation-created` indicates that relation data can be accessed; `relation-broken` indicates that relation data can no longer be read-written.

The hook indicates that the relation under consideration is no longer valid, and that the charm’s software must be configured as though the relation had never existed. It will only be called after every hook bound to `<endpoint>-relation-departed` has been run. If a hook bound to this event is being executed, it is guaranteed that no remote units are currently known locally.


It is important to note that the `relation-broken` hook might run even if no other units have ever joined the relation. This is not a bug: even if no remote units have ever joined, the fact of the unit’s participation can be detected in other hooks via the `relation-ids` hook command, and the `-broken` hook needs to execute to allow the charm to clean up any optimistically-generated configuration.

Also, it’s important to internalise the fact that there may be multiple relations in play with the same name, and that they’re independent: one `relation-broken` hook does not mean that *every* such relation is broken.

For a peer relation, `<peer endpoint name>-relation-broken` will never fire, not even during the teardown phase.

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA

-->

(hook-relation-changed)=
#### `<endpoint>-relation-changed`

*What triggers it?*

The `relation-changed` hook for a given unit always runs once immediately following the `relation-joined` hook for that unit, and subsequently whenever the related unit changes its settings (by calling `relation-set` and exiting without error). Note that immediately only applies within the context of this particular runtime relation -- that is, when `foo-relation-joined` is run for unit `bar/99` in relation id `foo:123`, the only guarantee is that/ the next hook to be run *in relation id `foo:123`* will be `foo-relation-changed` for `bar/99`. Unit hooks may intervene, as may hooks for other relations, and even for other foo relations.

`relation-changed` is emitted when another unit involved in the relation (from either side) touches the relation data. Relation data is *the* way for charms to share non-sensitive information (for sensitive information, `juju secrets` are on their way in Juju 3).

For centralized data -- for example, a single password or API token that one application generates to share with another application, we suggest that charm authors use the application data, rather than individual unit data. This data can only be written to by the application leader, and each remote unit related to that application will receive a single `relation-changed` event when it changes.

Hooks bound to this event should be the only ones that rely on remote relation settings. They should not error if the settings are incomplete, since it can be guaranteed that when the remote unit or application changes its settings again, this event will fire once more.

Charm authors should expect this event to fire many times during an application's life cycle. Units in an application are able to update relation data as needed, and a `relation-changed` event will fire every time the data in a relation changes. Since relation data can be updated on a per unit bases, a unit may receive multiple `relation-changed` events if it is related to multiple units in an application and all those units update their relation data.

This event is guaranteed to follow immediately after each {ref}`hook-relation-joined` hook. So all `juju` commands that trigger `relation-joined` will also cause `relation-changed` to be fired. So typical scenarios include:

|    Scenario    | Example Command              | Resulting Events                                                                                 |
|:--------------:|------------------------------|--------------------------------------------------------------------------------------------------|
| Add a relation | `juju integrate foo:a bar:b` | (all `foo` and `bar` units) <br> `*-relation-created -> *-relation-joined -> *-relation-changed` |

Additionally, a unit will receive a `relation-changed` event every time another unit involved in the relation changes the relation data. Suppose `foo` receives an event, and while handling it the following block executes:

```python
# in charm `foo`
relation.data[self.unit]['foo'] = 'bar'  # set unit databag
if self.unit.is_leader():
    relation.data[self.app]['foo'] = 'baz'  # set app databag
```

When the hook returns, `bar` will receive a `relation-changed` event.

Note that units only receive `relation-changed` events for **other** units' changes. This can matter in a peer relation, where the application leader will not receive a `relation-changed` event for the changes that it writes to the peer relation's application data bag. If all units, including the leader, need to react to a change in that application data, charm authors may include an inline `.emit()` for the `<name>_relation_changed` event on the leader.


> **When is data synchronized?** <br>
> Relation data is sent to the controller at the end of the hook's execution. If a charm author writes to local relation data multiple times during the a single hook run, the net change will be sent to the controller after the local code has finished executing. The controller inspects the data and determines whether the relation data has been changed. Related units then get the `relation-changed` event the next time they check in with the controller.


<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(hook-relation-created)=
#### `<endpoint>-relation-created`

*What triggers it?*

`relation-created` is a "setup" event and, emitted when an application is related to another. Its purpose is to inform the newly related charms that they are entering the relation.

If Juju is aware of the existence of the relation "early enough", before the application has started (i.e. *before* the application has started, i.e., before the {ref}`start hook <hook-start>` has run), this event will be fired as part of the setup phase. An important consequence of this fact is, that for all peer-type relations, since Juju is aware of their existence from the start, those `relation-created`  events will always fire before `start`.

Similarly, if an application is being scaled up, the new unit will see `relation-created` events for all relations the application already has during the Setup phase.


|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
| Integrate  | `juju integrate foo bar` | (all foo & bar units): `*-relation-created --> *-relation-joined -> *-relation-changed` |
|  Scale up an integrated app | `juju add-unit -n1 foo` | (new foo unit): `install -> *-relation-created -> config-changed -> start` |


In the following scenario, one deploys two applications and relates them "very early on". For example, in a single command.
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Deploy and quickly integrate | `juju deploy foo; juju deploy bar; juju integrate foo bar` | (all units): same as previous case. |

Starting from when `*-relation-created` is received, relation data can be read-written by units, up until when the corresponding `*-relation-broken` is received.

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA

-->

(hook-relation-departed)=
#### `<endpoint>-relation-departed`

*What triggers it?*

Emitted when a unit departs from an existing relation.

The `relation-departed` hook for a given unit always runs once when a related unit is no longer related. After the "relation-departed" hook has run,
no further notifications will be received from that unit; however, its settings will remain accessible via the `relation-get` hook command for the complete lifetime of the relation.


`relation-departed` is a {ref}`teardown <teardown-phase>` hook, emitted when a remote unit departs a relation.
This event is the exact inverse of `relation-joined`.


`*-relation-broken` events are emitted on a unit when a related application is scaled down. Suppose you have two related applications, `foo` and `bar`.
If you scale down `bar`, all `foo` units will receive a `*-relation-departed` event. The departing unit will receive a `*-relation-broken` event as part of its {ref}`teardown phase <teardown-phase>`.
Also removing a relation altogether will trigger `*-relation-departed` events (followed by `*-relation-broken`) on all involved units.

|     Scenario     | Example Command                      | Resulting Events                                                                |
|:----------------:|--------------------------------------|---------------------------------------------------------------------------------|
|   Unit removal   | `juju remove-unit --num-units 1 bar` | (foo/0): `*-relation-departed` <br> (bar/0): `*-relation-broken -> stop -> ...` |
| Relation removal | `juju remove-relation foo bar`       | (all units): `*-relation-departed -> *-relation-broken`                         |

Of course, removing the application altogether, instead of a single unit, will have a similar effect and also trigger these events.

Both `relation-departed` and `relation-broken` will always fire, regardless of how the relation is terminated.

```{note}

For a peer relation, the relation itself will never be destroyed until the application is removed and no units remain, at which point there won't be anything to call the `relation-broken` hook on anyway.

```

Note that on relation removal (`juju remove-relation`); only when all `-departed` hooks for such a relation and all callback methods bound to this event have been run for such a relation, the unit agent will fire `relation-broken`.

The `relation-departed` event is seen both by the leaving unit(s) and the remaining unit(s):

- For remaining units (those which have joined and not yet departed), this event is emitted once for each departing unit and in no particular order. At the point when a remaining unit receives a `relation-departed`, it's perfectly probable (although not guaranteed) that the system running that unit has already shut down.
- For departing units, this event is emitted once for each remaining unit.

A unit's relation settings persist beyond its own departure from the relation; the final unit to depart a relation marked for termination is responsible for destroying the relation and all associated data.

`relation-changed` is *not* fired for removed relations.
If you want to know when to remove a unit from your data, that would be `relation-departed`.

During a `relation-departed` hook, relation settings can still be read (with relation-get) and a relation can even still be set (with relation-set), by  explicitly providing the relation ID. All units will still be able to see all other units, and any unit can call the `relation-set` hook command to update their own published set of data on the relation. However, data updated by the departing unit will not be published to the remaining units. This is true even if there are no units on the other side of the relation to be notified of the change.

If any affected unit publishes new data on the relation during the `relation-departed` hooks, the new data will *not* be seen by the departing unit (it will *not* receive a `relation-changed` hook; only the remaining units will).


```{note}

(Juju internals) When a unit's own participation in a relation is known to be ending, the unit agent continues to uphold the guaranteed event ordering, but within those constraints, it will run the fewest possible hooks to notify the charm of the departure of each individual remote unit.

```

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(hook-relation-joined)=
#### `<endpoint>-relation-joined`

*What triggers it?*

`<endpoint name>-relation-joined`; emitted when a new unit joins in an existing relation.

The "relation-joined" hook always runs once when a related unit is first seen.


`relation-joined` is emitted when a unit joins in an existing relation. The unit will be a local one in the case of peer relations, a remote one otherwise.

By the time this event is emitted, the only available data concerning the relation is
 - the name of the joining unit.
 - the `private-address` of the joining unit.

In other words, when this event is emitted the remote unit has not yet had an opportunity to write any data to the relation databag. For that, you're going to have to wait for the first {ref}`relation-changed hook <hook-relation-changed>`.



From the perspective of an application called `foo`, which can relate to an application called `bar`:

|   Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju integrate foo bar` |  `*-relation-created -> *-relation-joined -> *-relation-changed` |
|  Create unit   | `juju add-unit bar -n 1`  |  `*-relation-joined -> *-relation-changed`|


For a peer relation, `<peer relation name>-relation-joined` will be received by peers some time after a new peer unit appears. (And during its setup, that new unit will receive a  `<peer relation name>-relation-created`).


For a peer relation, `<peer relation name>-relation-joined` will only be emitted if the scale is larger than 1. In other words, applications with a scale of 1 do not see peer relation joined/departed events.
**If you are using peer data as a means for persistent storage, then use peer `relation-created` instead**.

`relation-joined` can fire multiple times per relation, as multiple units can join, and is the exact inverse of `relation-departed`.
That means that if you consider the full lifecycle of an application, a unit, or a model, the net difference of the number of `*-relation-joined` events and the number of `*-relation-departed` events will be zero.


<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(secret-hooks)=
### Secret hooks

Secret hooks are used to:
* inform the owner of a secret that a revision should be removed because it has expired.
* inform the owner of a secret that it should be rotated because it has become stale.
* inform the owner of a secret that unused revisions can safely be removed.
* inform the consumer of a secret that a new revision is available.

Secret hooks operate in an environment with additional environment variables available:

* `JUJU_SECRET_ID` holds the ID of the secret.
* `JUJU_SECRET_LABEL` holds the label of the secret.
* `JUJU_SECRET_REVISION` holds the revision of the secret.

The `secret-expired` hook is triggered when the secret's expiry time has been reached and the expired secret revision should be removed.

The `secret-rotate` hook is triggered when a secret has become stale at the end of its rotation interval and a new revision should be created.

The `secret-remove` hook is triggered when there are revisions of a secret that are no longer being tracked by any consumers and can be safely removed.

The `secret-changed` hook is triggered when there is a new revision available for a previously consumed secret.

(hook-secret-changed)=
#### `secret-changed`

*What triggers it?*

When a charm reads the content of a secret to which it has been granted access, it becomes a consumer of that secret.
Having become a secret consumer, a charm receives the `secret-changed` event when the secret owner publishes a new secret revision.
This event gives the consuming charm the chance to read the updated content of the latest secret revision.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `JUJU_SECRET_ID` holds the ID of the secret that has a new revision.
* `JUJU_SECRET_LABEL` holds the label given to the secret by the consumer.

The secret label can be set whenever a secret's content is read by passing `--label <label>` to the `secret-get` command.
Thereafter, any subsequent `secret-changed` hooks will be run with this label.

*Who gets it*?

All units observing the affected secret.

```{note}
Upon receiving that event (or at any time after that) a consumer can choose to:

 - Start tracking the latest revision by passing `--refresh` to the `secret-get` command.
 - Inspect the latest revision value, without tracking it just yet by passing `--peek` to the `secret-get` command.

 Without `--refresh` or `--peek`, the `secret-get` command continues to return the previously tracked revision, which may not be the latest.

Once all consumers have stopped tracking a specific outdated revision, the owner will receive a `secret-remove` hook to be notified of that fact, and can then remove that revision.
```

(hook-secret-expired)=
#### `secret-expired`

```{versionadded} 3.0.2
```
```{important}
Currently supported only for {ref}`charm secrets <charm-secret>`.
```

*What triggers it?*

Secret expiration enables the charm to define a moment in time when the given secret is obsoleted and should effectively stop working.
At that moment, Juju will inform the charm via the secret-expired hook that the given secret is expiring, at which point the charm has a chance to remove that particular secret revision from the model with the `secret-remove` command.

Until the charm indeed removes the expired secret revision, Juju will continue to regularly call the `secret-expired` hook to notify that the secret revision is supposed to be dropped.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `JUJU_SECRET_ID` holds the ID of the secret that is expiring.
* `JUJU_SECRET_REVISION` holds the revision that is expiring.
* `JUJU_SECRET_LABEL` holds the label given to the secret by the owner.

*Who gets it*?

The secret owner. For application owned secrets, the owner is the unit leader.

(hook-secret-remove)=
#### `secret-remove`
```{important}
Currently supported only for charm secrets.
```

*Who gets it*?

The secret owner. For application owned secrets, the owner is the unit leader.

*What triggers it?*

A secret revision no longer having any consumers and thus being safe to remove.

This situation can occur when:

- All consumers tracking a now-outdated revision have updated to tracking a newer one, so the old revision can be removed.
- No consumer is tracking an intermediate revision, and a newer one has already been created. So there is a orphaned revision which no consumer will ever be able to peek or update to, because there is already a newer one the consumer would get instead.

In short, the purpose of this event is to notify the owner of a secret that a specific revision of it is safe to remove: no charm is presently observing it or ever will be able to in the future.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:


* `JUJU_SECRET_ID` holds the ID of the secret that is expiring.
* `JUJU_SECRET_REVISION` holds the revision that can be removed.
* `JUJU_SECRET_LABEL` holds the label given to the secret by the owner.

(hook-secret-rotate)=
#### `secret-rotate`
```{versionadded} 3.0.2
```
```{important}
Currently supported only for charm secrets.
```

*What triggers it?*

Secret rotation enables the charm to define a moment in time when the given secret becomes stale and should be updated to generate a new revision.
At that moment, Juju will inform the charm via the `secret-rotate` hook that the given secret is stale, at which point the charm has a chance to add a new secret revision to the model with the `secret-set` command.
Until the charm updates the secret to create a new revision, Juju will continue to regularly call the `secret-rotate` hook to notify that the secret is supposed to be updated.

The secret rotation interval is specified using a named rotation policy, one of: `never`, `hourly`, `daily`, `weekly`, `monthly`,
`quarterly`, `yearly`.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `JUJU_SECRET_ID` holds the ID of the secret that needs rotation.
* `JUJU_SECRET_LABEL` holds the label given to the secret by the owner.

*Who gets it*?

The secret owner. For application owned secrets, the owner is the unit leader.

(storage-hooks)=
### Storage hooks

Storage hooks are used to inform a charm about changes to storage attached to its unit.

The hook names that these kinds represent will be prefixed by the storage name; for example, `database-storage-attached`.

For every storage defined by a charm, storage hook events are named after the storage name:

* `<storage_name>-storage-attached`
* `<storage_name>-storage-detaching`

For each charm storage, any or all of the above storage hooks can be implemented.

Storage hooks operate in an environment with additional environment variables available:

* `JUJU_STORAGE_ID` holds the ID of the storage.
* `JUJU_STORAGE_LOCATION` holds the path at which the storage is mounted.
* `JUJU_STORAGE_KIND` holds the kind of storage (`block` or `filesystem`).

The `storage-attached` hook is triggered when new storage is available for the charm to use.


For machine charms, all `storage-attached` hooks will be run before the `install` event fires.
This means that if any errors are encountered whilst performing the storage provisioning and attach operations, the charm will
not receive the `install` event until such errors have been resolved.

For Kubernetes charms, any `storage-attached` hooks are run some time after the `start` hook has completed.

The `storage-detaching` hook is triggered after the `stop` hook has completed and all such hooks will be run before triggering the `remove` hook.

(hook-storage-attached)=
#### `<storage>-storage-attached`

*What triggers it?*

A storage volume having been attached to the charm's host machine or container and being ready to be interacted with.

<!--
| `<name>_storage_attached`  | [`StorageAttachedEvents`](https://ops.readthedocs.io/en/latest/reference/ops.html#ops.StorageAttachedEvent)  | This event is triggered when new storage is available for the charm to use. Callback methods bound to this event allow the charm to run code when storage has been added.<br/><br/>Such methods will be run before the `install` event fires, so that the installation routine may use the storage. The name prefix of this hook will depend on the storage key defined in the `metadata.yaml` file.                            |
| `<name>_storage_detaching` | [`StorageDetachingEvent`](https://ops.readthedocs.io/en/latest/reference/ops.html#ops.StorageDetachingEvent) | Callback methods bound to this event allow the charm to run code before storage is removed.<br/><br/>Such methods will be run before storage is detached, and always before the `stop` event fires, thereby allowing the charm to gracefully release resources before they are removed and before the unit terminates.<br/><br/>The name prefix of the hook will depend on the storage key defined in the `metadata.yaml` file. |
-->

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(hook-storage-detaching)=
#### `<storage>-storage-detaching`

*What triggers it?*

A request to detach storage having been processed.

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(workload-hooks)=
### Workload (Pebble) hooks

> Currently Kubernetes only.

Workload (Pebble) hooks are used to inform the charm about events related to a Pebble managed workload.

The hook names that these kinds represent will be prefixed by the workload/container name; for example, `mycontainer-pebble-ready`.

Workload hooks operate in an environment with additional environment variables available:

* `JUJU_WORKLOAD_NAME` holds the name of the container to which the hook pertains.
* `JUJU_NOTICE_ID` holds the Pebble notice ID.
* `JUJU_NOTICE_TYPE` holds the Pebble notice type.
* `JUJU_NOTICE_KEY` holds the Pebble notice key.
* `JUJU_PEBBLE_CHECK_NAME` holds the name of the Pebble check.

(hook-container-pebble-check-failed)=
#### `<container>-pebble-check-failed`
```{important}
Kubernetes sidecar charms only.
```

```{versionadded} 3.6
```

*What triggers it?*

A Pebble check passing the failure threshold.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `$JUJU_WORKLOAD_NAME` holds the name of the container to which the hook pertains.
* `$JUJU_PEBBLE_CHECK_NAME` holds the name of the Pebble check.

*Who gets it*?

The charm responsible for the container to which the hook pertains.

(hook-container-pebble-check-recovered)=
#### `<container>-pebble-check-recovered`
```{important}
Kubernetes sidecar charms only.
```

```{versionadded} 3.6
```

*What triggers it?*

A Pebble check passing after previously reaching the failure threshold.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `$JUJU_WORKLOAD_NAME` holds the name of the container to which the hook pertains.
* `$JUJU_PEBBLE_CHECK_NAME` holds the name of the Pebble check.

*Who gets it*?

The charm responsible for the container to which the hook pertains.

(hook-container-pebble-custom-notice)=
#### `<container>-pebble-custom-notice`
> Kubernetes sidecar charms only

*What triggers it?*

A Pebble notice of type "custom" occurring.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>` and:

* `JUJU_WORKLOAD_NAME` holds the name of the container to which the hook pertains.
* `JUJU_NOTICE_ID` holds the Pebble notice ID.
* `JUJU_NOTICE_TYPE` holds the Pebble notice type.
* `JUJU_NOTICE_KEY` holds the Pebble notice key.

*Who gets it*?

The charm responsible for the container to which the hook pertains.

(hook-container-pebble-ready)=
#### `<container>-pebble-ready`
```{important}
Kubernetes sidecar charms only.
```

*What triggers it?*

The requested container being ready.

The `pebble-ready` event doesn't guarantee the workload container is *still* up. For example, if you manually `kubectl patch` during (for example) `install`, then you may receive this event after the old workload is down but before the new one is up.
For this reason it's essential, even in `pebble-ready` event handlers, to catch [`ConnectionError`](https://ops.readthedocs.io/en/latest/reference/pebble.html#ops.pebble.ConnectionError) when using Pebble to make container changes. There is a [`Container.can_connect`()](https://ops.readthedocs.io/en/latest/reference/ops-testing.html#ops.testing.Container.can_connect) method, but note that this is a point-in-time check, so just because `can_connect()` returns `True` doesn’t mean it will still return `True` moments later. So, **code defensively** to avoid race conditions.

Moreover, as pod churn can occur at any moment, `pebble-ready` events can be received throughout any phase of a charm's lifecycle. Each container could churn multiple times, and they all can do so independently from one another. In short, the charm should make no assumptions at all about the moment in time at which it may or may not receive `pebble-ready` events, or how often that can occur. The fact that the charm receives a `pebble-ready` event indicates that the container has just become ready (for the first time, or again, after pod churn), therefore you typically will need to **reconfigure your workload from scratch** every single time.

This feature of `pebble-ready` events make them especially suitable for a [holistic handling pattern](https://ops.readthedocs.io/en/latest/explanation/holistic-vs-delta-charms.html).


*Which environment variables is it executed with?*

* `$JUJU_WORKLOAD_NAME` holds the name of the container to which the hook pertains.

*Who gets it*?

The charm responsible for the container to which the hook pertains.

<!--

<a href="#heading--relation-event-triggers"><h2 id="heading--relation-event-triggers">Relation event triggers</h2></a>

Relation events trigger as a response to changes in the Juju model relation topology. When a new relation is created or removed, events are fired on all units of both involved applications.

|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | ---------------------------------------- | ------------------------------------ |
|  Relate   | `juju integrate foo:a bar:b`     | `(foo): a-relation-created -> a-relation-changed`<br> `(bar): b-relation-created -> b-relation-changed` |
|  Remove relation   | `juju remove-relation foo:a bar:b`     | `(foo): a-relation-broken`<br> `(bar): b-relation-broken` |

If you have two already related applications, and one of them gains or loses a unit, then the newly added unit will receive the same event sequences as if it had just been related (from its point of view, the relation is 'brand new'), while the units that were there already receive a `-relation-joined` event.
Similarly if a unit is removed, that unit will receive `-relation-broken`, while the ones that remain will see a `-relation-departed`.

|  Scenario   | Example Command                          | Resulting Events                     |
| :-------: | ---------------------------------------- | ------------------------------------ |
|  Add unit   | `juju add-unit foo -n 1`     | `(foo): a-relation-created -> a-relation-changed`<br> `(bar): b-relation-joined -> b-relation-changed` |
|  Remove relation   | `juju remove-unit foo:a --num-units 1`     | `(foo): a-relation-broken`<br> `(bar): b-relation-departed` |

```{note}

`-relation-changed` events are not only fired as part of these event sequences, but also whenever a unit touches the relation data.
As such, contrary to many other events, `-relation-changed` events are mostly triggered by charm code (and not by the cloud admin doing things on the Juju model).

```
-->

(other-hooks)=
### Other hooks

(hook-config-changed)=
#### `config-changed`

*What triggers it?*

The `config-changed` hook always runs once immediately after the initial `install` hook and likewise after the `upgrade-charm` hook.

It also runs whenever:
* application configuration changes.
* when any IP address of the machine hosting the unit changes.
* when the trust value of the application changes.
* when recovering from transient unit agent errors.

*Which environment variables is it executed with?*

All the {ref}`generic environment variables <generic-environment-variables>`.

*Who gets it*?

When application config or trust has changed, all the units of an application for which the config has changed.
When the networking on a machine has changed, any units deployed to that machine.


<!--
#### Pebble hooks
> These hooks require an associated workload/container, and the name of the workload/container whose change triggered the hook. The hook file names that these kinds represent will be prefixed by the workload/container name; for example, `mycontainer-pebble-ready`.
-->



(hook-install)=
#### `install`

*What triggers it?*

The `install` hook always runs once, and only once, before any other hook.

fired when Juju is done provisioning the unit.

The `install` event is emitted once per unit at the beginning of a charm's lifecycle. Associated callbacks should be used to perform one-time initial setup operations and prepare the unit to execute the application. Depending on the charm, this may include installing packages, configuring the underlying machine or provisioning cloud-specific resources.

Therefore, ways to cause `install` to occur are:
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo`<br>`juju add-unit foo`  | `install -> config-changed`|


Note:
- Typically, operations performed on {ref}`hook-install` should also be considered for {ref}`hook-upgrade-charm`.
- In some cases, the {ref}`config-changed <hook-config-changed>` hook  can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.

The `install` event is emitted once per unit at the beginning of a charm's lifecycle. Associated callbacks should be used to perform one-time initial setup operations and prepare the unit to execute the application. Depending on the charm, this may include installing packages, configuring the underlying machine or provisioning cloud-specific resources.

Therefore, ways to cause `install` to occur are:
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo`<br>`juju add-unit foo`  | `install -> config-changed`|


Note:
- Typically, operations performed on `install` should also be considered for {ref}`hook-upgrade-charm`.
- In some cases, {ref}`hook-config-changed` can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->


(hook-remove)=
#### `remove`

*What triggers it?*

The `remove` event is emitted only once per unit: when the Juju controller is ready to remove the unit completely. The `stop` event is emitted prior to this, and all necessary steps for handling removal should be handled there.

On Kubernetes charms, the `remove` event will occur on pod churn, when the unit dies. On machine charms, the stop event will be fired when a unit is put down.

|   Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Remove unit   | `juju remove-unit foo/0` (on machine) or <br> `juju remove-unit --num-units 1 foo` (on Kubernetes)  | `stop -> [relation/storage teardown] -> remove` |

Of course, removing an application altogether will result in these events firing on all units.

If the unit has any relations active or any storage attached at the time the removal occurs, these will be cleaned up (in no specific order) between `stop` and `remove`. This means the unit will receive `stop -> (*-relation-broken | *-storage-detaching) -> remove`.

The `remove` event is the last event a unit will ever see before going down, right after {ref}`hook-stop`. It is exclusively fired when the unit is in the {ref}`teardown phase <teardown-phase>`.

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->




(hook-start)=
#### `start`

*What triggers it?*

A unit's initialization being complete.

This can occur:

- when the unit is being created
- (Kubernetes:) on pod churn
- (Kubernetes:) on cluster reboot
- on charm upgrades

For Kubernetes charms, this occurs on pod churn as well.

Callback methods bound to the event should be used to ensure that the charm’s software is in a running state. Note that the charm’s software should be configured to persist in a started state without further intervention from Juju or an administrator.

In Kubernetes sidecar charms, Juju provides no ordering guarantees regarding `start` and `*-pebble-ready`.

*Which hooks can be guaranteed to have fired before it, if any?*?

The `config-changed` hook. (The `start` hook is fired immediately after.)

*Which environment variables is it executed with?*

*Who gets it*?

Any unit.

(hook-stop)=
#### `stop`

*What triggers it?*

The Juju controller being ready to destroy the unit.

This can occur:

- when the unit is being removed (whether explicitly or through the application as a whole being removed)
- (Kubernetes:) on pod churn


The `stop` hook is the  one-before-last  hook the unit will receive before being destroyed (the last one being `remove`).

```{note}
 When handling the `stop` event, charms should gracefully terminate all services for the supported application and update any relevant cluster/leader information to remove or update any data relating to the current unit. Additionally, the charm should ensure that the software will not automatically start again on reboot.

```

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*
TBA
-->

*Who gets it*?

Any unit.





(hook-update-status)=
#### `update-status`

*What triggers it?*

Nothing in particular -- this hooks is fired automatically by Juju at regular intervals (default: 5m; can be changed, e.g., `juju model-config update-status-hook-interval=1m`).

This event can be used to monitor the health of deployed charms and determine the status of long running tasks (such as package installation), updating the status message reported to Juju accordingly.

Historically, this hook was intended to allow authors to run code that gets the “health” of the application. However, health checks can also be specified via {ref}`pebble`.

Since the update-status interval is model-wide (not per application) and is set by the user (for example, it can be set to once per hour), charms should not rely on it for critical operations.

In integration tests, unless specifically testing the update-status hook, you may want to "disable" it so it doesn't interfere with the test. This can be achieved by setting the interval to e.g. 1h at the beginning of the test.

*Which hooks can be guaranteed to have fired before it, if any?*

As it is triggered periodically, the `update-status`  can happen in-between any other charm events.

By default, the `update-status` event is triggered by the Juju controller at 5-minute intervals.

<!--
*Which environment variables is it executed with?*

TBA

*Who gets it*?

TBA
-->

(hook-upgrade-charm)=
#### `upgrade-charm`

*What triggers it?*

The `upgrade-charm` hook always runs once immediately after the charm directory contents have been changed by an unforced charm upgrade operation, and *may* do so after a forced upgrade; but will *not* be run after a forced upgrade from an existing error state. (Consequently, neither will the `config-changed` hook that would ordinarily follow the `upgrade-charm`.


The event is emitted after the new charm code has been unpacked - therefore this event is handled by the callback method bound to the event in the new codebase.

The associated callback should be used to reconcile the current state written by an older version into whatever form is required by the current charm version. An example of a reconciliation that needs to take place is to migrate an old relation data schema to a new one.

```{important}

- Typically, operations performed on `upgrade-charm` should also be considered for {ref}`hook-install`.
- In some cases, the {ref}`hook-config-changed` hook can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.
- Note that you cannot upgrade a Charmhub charm to the same version. However, upgrading a local charm from path works (and goes through the entire upgrade sequence) even if the charm is exactly the same.

```

|  Scenario | Example command | Resulting events |
|:-:|-|-|
|Upgrade Kubernetes charm | `juju refresh` | `stop` (old charm) -> `upgrade-charm` (new charm) -> `config-changed` -> `start` -> `*-pebble-ready`|
|Upgrade machine charm | `juju refresh` | `upgrade-charm` -> `config-changed` -> `start`|
|Attach resource | `juju attach-resource foo bar=baz` | (same as upgrade) |

```{important}

An upgrade does NOT trigger any relation hooks (unless relation data is intentionally modified in one of the upgrade sequence hooks).
```

<!--
*Which hooks can be guaranteed to have fired before it, if any?*?

TBA

*Which environment variables is it executed with?*

TBA
-->

*Who gets it*?

Any unit.

## Hook and action environment variables

### `<environment variable(s) specific to a hook (kind) or action>`

See the documentation for the relevant hook or action.

(generic-environment-variables)=
### Generic environment variables

* `PATH` is the usual Unix path, prefixed by a directory containing command line tools through which the hooks can interact with Juju.
* `JUJU_CHARM_DIR` holds the path to the charm directory.
* `JUJU_HOOK_NAME` holds the name of the currently executing hook.
* `JUJU_UNIT_NAME` holds the name of the local unit.
* `JUJU_CONTEXT_ID`, `JUJU_AGENT_SOCKET_NETWORK` and `JUJU_AGENT_SOCKET_ADDRESS` are used by the hook commands to connect to the local agent.
* `JUJU_API_ADDRESSES` holds a space separated list of Juju API addresses.
* `JUJU_MODEL_UUID` holds the UUID of the current model.
* `JUJU_MODEL_NAME` holds the human friendly name of the current model.
* `JUJU_PRINCIPAL_UNIT` holds the name of the principal unit if the current unit is a subordinate.
* `JUJU_MACHINE_ID` holds the ID of the machine on which the local unit is running.
* `JUJU_AVAILABILITY_ZONE` holds the cloud's availability zone where the machine has been provisioned.
* `CLOUD_API_VERSION` holds the API version of the cloud endpoint.
* `JUJU_VERSION` holds the version of the agent for the local unit.
* `JUJU_CHARM_HTTP_PROXY` holds the value of the `juju-http-proxy` model config attribute.
* `JUJU_CHARM_HTTPS_PROXY` holds the value of the `juju-https-proxy` model config attribute.
* `JUJU_CHARM_FTP_PROXY` holds the value of the `juju-ftp-proxy` model config attribute.
* `JUJU_CHARM_NO_PROXY` holds the value of the `juju-no-proxy` model config attribute.