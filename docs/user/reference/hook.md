---
tocdepth: 3
---

(hook)=
# Hook

In Juju, a **hook** is a notification from  the controller agent through the unit agent to the charm that the internal representation of Juju has changed in a way that requires a reaction from the charm so that the unit's state and the controller's state can be reconciled.


For a charm written with {ref}`Ops <ops>`, Juju hooks are translated into Ops events = 'events', specifically, into classes that inherit from [`HookEvent`](https://ops.readthedocs.io/en/latest/index.html#ops.HookEvent).

<!--
(charm-lifecycle)=
## Charm lifecycle

This document is about the lifecycle of a charm, specifically the Juju events that are used to keep track of it. These events are relayed to charm code by the Operator Framework in specific sequences depending on what's going on in the Juju model. 

It is common wisdom that event ordering should not be generally relied upon when coding a charm, to ensure resilience. It can be however useful to understand the logic behind the timing of events, so as to avoid common mistakes and have a better picture of what is happening in your charm. In this document we'll learn how:

* A charm's lifecycle can be seen to consist of three **phases**, each one with characteristic events and sequences thereof. The fuzziest of the three being the Operation phase, where pretty much anything can happen short of setup events.
* Not all events can be reliably be assumed to occur in specific temporal orders, but some can.

In this document we will *not* learn:

* What each event means or is typically used to represent about a workload status. For that see [the SDK docs](https://juju.is/docs/sdk/events). 
* What event cascades are triggered by a human administrator running commands through the Juju CLI. For that see [this other doc](https://discourse.charmhub.io/t/core-lifecycle-events/4455/3).

```{note}

The graphs are screenshots of mermaid sources currently available [here](https://github.com/PietroPasotti/charm-events), pending mermaid support to be available on discourse.

```


### The graph

![The graph](hook-charm-lifecycle-1.png)

#### Legend

* `(start)` and `(end)` are 'meta' nodes and represent the beginning and end of the lifecycle of a Charm/juju unit. All other nodes represent hooks (events) that can occur during said lifecycle.
* Hard arrows represent strict temporal ordering which is enforced by the Juju state machine and respected by the Operator Framework, which mediates between the Juju controller and the Charm code.
* Dotted arrows represent a 1:1 relationship between relation events, explained in more detail down in the Operation section.
* The large yellow boxes represent broad phases in the lifecycle. You can read the graph as follows: when you fire up a unit, there is first a setup phase, when that is done the unit enters a operation phase, and when the unit goes there will be a sequence of teardown events. Generally speaking, this guarantees some sort of ordering of the events: events that are unique to the teardown phase can be guaranteed not to be fired during the setup phase. So a `stop` will never be fired before a `start`.
* The colours of the event nodes represent a logical but practically meaningless grouping of the events.
  * green for leadership events
  * red for storage events
  * purple for relation events
  * blue for generic lifecycle events   

#### Workload and substrate-specific events

Note the `[workload events] (k8s only)` node in the operation phase. That represents all events meant to communicate information about the workload container on kubernetes charms. At the time of writing the only such events are:

* [`*-pebble-ready` <event-container-pebble-ready>`
* {ref}``*-pebble-custom-notice` <event-container-pebble-custom-notice>`
* [`*-pebble-check-failed`](https://ops.readthedocs.io/en/latest/#ops.PebbleCheckFailedEvent)
* [`*-pebble-check-recovered`](https://ops.readthedocs.io/en/latest/#ops.PebbleCheckRecoveredEvent)

All of these can fire at any time whatsoever during the lifecycle of a charm.

Similarly, the `{ref}`pre/post]-series-upgrade (lxd only)` events can only occur on machine charms at any time during the operation phase.

## Notes on the setup phase
* The only events that are guaranteed to always occur during Setup are [`start`], [`config-changed`] and [`install`]. The other events only happen if the charm happens to have (peer) relations at install time (e.g. if a charm that already is related to another gets scaled up) or it has storage. Same goes for leadership events. For that reason they are styled with dashed borders.
* [`config-changed`] occurs between [`install`] and [`start`] regardless of whether any leadership (or relation) event fires.
* Any [`*-relation-created`] event can occur at Setup time, but if X is a peer relation, then `X-relation-created` can **only** occur at Setup, while for non-peer relations, they can occur also during Operation. The reason for this is that a peer relation cannot be created or destroyed 'manually' at arbitrary times, they either exist or not, and if they do exist, then we know it from the start.

### Notes on the operation phase

* [`update-status` <event-update-status>` is fired automatically and periodically, at a configurable regular interval (default is 5m) which can be configured by `juju model-config update-status-hook-interval`.
`collect-metrics` is fired automatically and periodically in older juju versions, at a regular interval of 5m, AND whenever the user runs `juju collect-metrics`.
* [`leader-elected`] and [`leader-settings-changed`] only fire on the leader unit and the non-leader unit(s) respectively, just like at startup.
* There is a square of symmetries between the `*-relation-[joined/departed/created/broken]` events:
  * Temporal ordering: a `X-relation-joined` cannot *follow* a `X-relation-departed` for the same relation ID. Same goes for [`*-relation-created`] and [`*-relation-broken`], as well as [`*-relation-created`] and [`*-relation-changed`].
  * Ownership: `joined/departed` are unit-level events: they fire when an application has a (peer) relation and a new unit joins or leaves. All units (including the newly created or leaving unit), will receive the event. `created/broken` are relation-level events, in that they fire when two applications become related or a relation is removed (e.g. via `juju remove-relation` or because an application is destroyed).
  * Number: there is a 1:1 relationship between `joined/departed` and `created/broken`: when a unit joins a relation with X other units, X [`*-relation-joined`] events will be fired. When a unit leaves, all units will receive a [`*-relation-departed`] event (so X of them are fired). Same goes for `created/broken` when two applications are related or a relationship is broken. Find in appendix 1 a somewhat more elaborate example.
* Technically speaking all events in this box are optional, but I did not style them with dashed borders to avoid clutter. If the charm shuts down immediately after start, it could happen that no operation event is fired.
* A `X-relation-joined` event is always followed up (immediately after) by a `X-relation-changed` event. But any number of [`*-relation-changed`] events can be fired at any time during operation, and they need not be preceded by a [`*-relation-joined`] event.
* There are more temporal orderings than the one displayed here; event chains can be initiated by human operation as detailed [in the SDK docs](https://juju.is/docs/sdk/events) and [the leadership docs](https://juju.is/docs/sdk/leadership). For example, it is guaranteed that a [`leader-elected`] is always followed by a [`settings-changed`], and that if you remove the leader unit, you should get [`*-relation-departed`] and a [`leader-settings-changed`] on the remaining units (although no specific ordering can be guaranteed [cfr this bug...](https://bugs.launchpad.net/juju/+bug/1964582)). 
* Secret events (in purple) can technically occur at any time, provided your charm either has created a secret, or observes a secret that some other charm has created. Only the owner of a secret can receive `secret-rotate` and `secret-expire` for that secret, and only an observer of a secret can receive `secret-changed` and `secret-removed`. 

### Notes on the teardown phase

* Both relation and storage events are guaranteed to fire before [`stop`]/[`remove`] if they will fire at all. They are optional, in that a departing unit (or application) might have no storage or relations.
* [`*-relation-broken`] events in the Teardown phase are fired in case an application is being torn down. These events can also occur at Operation time, if the relation is removed by e.g. a charm or a controller.
* The entire teardown phase is **skipped** if the cloud is killed. The next event the charm will see in this case would be a `start` event. This would happen, for example, on `microk8s stop; microk8s start`.

### Caveats

* Events can be deferred by charm code by calling `Event.defer()`. That means that the event is put in a queue of deferred events which will get flushed by the operator framework as soon as the next event comes in, and *before* firing that new event in turn. See Appendix 2 for a visual representation. What this means in practice is that deferring an event can break the temporal ordering of the events as outlined in this graph; `defer()`ring an event twice will break the ordering guarantees we outlined here. Cf. the appendix for an UML-y representation. Cfr [this document on defer](https://discourse.charmhub.io/t/deferring-events-details-and-dilemmas/5930) for more.
* The events in the Operation phase can interleave in arbitrary ways. For this reason it's essential that hook handlers make *no assumptions* about each other -- each handler should check its preconditions independently and operate under the assumption that the relative ordering is totally arbitrary -- except relation events, which have some partial ordering as explained above.

### Deprecation notices

* `leader-deposed` is a juju hook that was planned but never actually implemented. You may see a WARNING mentioning it in the `juju debug-log` but you can ignore it.
* [`collect-metrics`](https://discourse.charmhub.io/t/charm-hooks/1040#heading--collect-metrics) is no longer being fired in recent juju versions.

### Event semantics and data

This document is only about the timing of the events; for the 'meaning' of the events, other sources are more appropriate; e.g. [juju-events](https://juju.is/docs/sdk/events).
For the data attached to an event, one should refer to the docstrings in the ops.charm.HookEvent subclass that the event you're expecting in your handler inherits from.


## Appendices

### Appendix 1: scenario example


This is a representation of the relation events a deployment will receive in a simple scenario that goes as follows:
* We start with two unrelated applications, `applicationA` and `applicationB`, with one unit each.
* `applicationA` and `applicationB` become related via a relation called `R`.
* `applicationA` is scaled up to 2 units.
* `applicationA` is scaled down to 1 unit.
* `applicationA` touches the `R` databag (e.g. during an `update-status` hook, or as a result of a `config-changed`, an action, a custom event...).
* The relation `R` is removed.

Note that many event sequences are marked as 'par' for *parallel*, which means that the events can be dispatched to the units arbitrarily interleaved.

![Charm lifecycle](hook-charm-lifecycle-2a.png)
![Charm lifecycle](hook-charm-lifecycle-2b.png)

### Appendix 2: deferring an event

 > {ref}`jhack tail <explore-event-emission-with-jhack-tail>` offers functionality to visualize the deferral status of events in real time.

This is the 'normal' way of using `defer()`: an event `event1` comes in but we are not ready to process it; we `defer()` it; when `event2` comes in, the operator framework will first flush the queue and fire `event1`, then fire `event2`. The ordering is preserved: `event1` is consumed before `event2` by the charm.

![Charm lifecycle](hook-charm-lifecycle-3.png)

Suppose now that the charm defers `event1` again; then `event2` will be processed by the charm before `event1` is. `event1` will only be fired again once another event, `event3`, comes in in turn.
The result is that the events are consumed in the order: `2-1-3`. Beware.

![Charm lifecycle](hook-charm-lifecycle-4.png)


-->



## List of hooks

> [Source](https://github.com/juju/juju/blob/main/internal/charm/hooks/hooks.go)

(hook-action-action)=
### `<action>-action`

#### What triggers it?

A charm user invoking the action name from the Juju CLI (`juju run <unit/0> foo`,  `juju run <unit/0> <unit/1> foo`).

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

All the units that the charm user has run the action on.

<!--
> Note: If hyphens are used in action names, they are replaced with underscores in the corresponding event names. For example, an action named `snapshot-database` would result in an event named `snapshot_database_action` being triggered when the action is invoked.

> Note that the action handler in the charm can in principle cause other events to be fired. For example:
> - Deferred events will trigger before the action.
> - If the action handler updates relation data, a `<relation name>_relation_changed` will be emitted afterwards on the affected units.
-->

(hook-collect-metrics)=
### `collect-metrics` (deprecated)

<!--
#### What triggers it?

#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?
-->

Exposes metrics for external monitoring.

(hook-config-changed)=
#### `config-changed`

#### What triggers it?

The `config-changed` hook always runs once immediately after the `install` hook, and likewise after the `upgrade-charm` hook. It also runs whenever the service configuration changes, and when recovering from transient unit agent errors.

The `config-changed` event is emitted in response to various events throughout a charm’s lifecycle:

- In response to a configuration change using the GUI or CLI.
- On networking changes (if the machine reboots and comes up with a different IP).
- Some time between the `install` event and the `start` event in the {ref}`startup phase of a charm's lifecycle <charm-lifecycle>`. <br>(The `config_changed` event will **ALWAYS** happen at least once, when the initial configuration is accessed from the charm.)

Callbacks associated with this event should ensure the current charm configuration is properly reflected in the underlying application configuration. Invocations of associated callbacks should be idempotent and should not make changes to the environment, or restart services, unless there is a material change to the charm's configuration, such as a change in the port exposed by the charm, addition or removal of a relation which may require a database migration or a "scale out" event for high availability, or similar.

Callbacks must not assume that the underlying applications or services have been started.

There are many situations in which `config-changed` can occur. In many of them,  the event being fired does not mean that the config has in fact changed, though it may be useful to execute logic that checks and writes workload configuration. For example, since `config-changed` is guaranteed to fire once during the startup sequence, some time after `install` is emitted, charm authors might omit a call to write out initial workload configuration during the `install` hook, relying on that configuration to be written out in their `config-changed` handler instead.

|  Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo`<br>`juju add-unit foo`  | `install -> config-changed -> start` |
|  Configure a unit   | `juju config foo bar=baz`  | `config-changed` |


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA


#### Who gets it?

TBA


<!--
#### Pebble hooks
> These hooks require an associated workload/container, and the name of the workload/container whose change triggered the hook. The hook file names that these kinds represent will be prefixed by the workload/container name; for example, `mycontainer-pebble-ready`.
-->

(hook-container-pebble-change-updated)=
###  `<container>-pebble-change-updated` 

#### What triggers it?

#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?

(hook-container-pebble-check-failed)=
### `<container>-pebble-check-failed`
> Kubernetes sidecar charms only.
>
> Added in Juju 3.6.

#### What triggers it?

A Pebble check passing the failure threshold.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?


(hook-container-pebble-check-recovered)=
### `<container>-pebble-check-recovered`
> Kubernetes sidecar charms only.
>
> Added in Juju 3.6.

#### What triggers it?

A Pebble check passing after previously reaching the failure threshold.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-container-pebble-custom-notice)=
### `<container>-pebble-custom-notice`
> Kubernetes sidecar charms only

#### What triggers it?

A Pebble notice of type "custom" occurring.

#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?

(hook-container-pebble-ready)=
### `<container>-pebble-ready`
> Kubernetes sidecar charms only.

#### What triggers it?

The requested container being ready.

The `pebble-ready` event doesn't guarantee the workload container is *still* up. For example, if you manually `kubectl patch` during (for example) `install`, then you may receive this event after the old workload is down but before the new one is up.
For this reason it's essential, even in `pebble-ready` event handlers, to catch [`ConnectionError`](https://ops.readthedocs.io/en/latest/pebble.html#ops.pebble.ConnectionError) when using Pebble to make container changes. There is a [`Container.can_connect`()](https://ops.readthedocs.io/en/latest/#ops.Container.can_connect) method, but note that this is a point-in-time check, so just because `can_connect()` returns `True` doesn’t mean it will still return `True` moments later. So, **code defensively** to avoid race conditions.

Moreover, as pod churn can occur at any moment, `pebble-ready` events can be received throughout any phase of [a charm's lifecycle](https://juju.is/docs/sdk/a-charms-life). Each container could churn multiple times, and they all can do so independently from one another. In short, the charm should make no assumptions at all about the moment in time at which it may or may not receive `pebble-ready` events, or how often that can occur. The fact that the charm receives a `pebble-ready` event indicates that the container has just become ready (for the first time, or again, after pod churn), therefore you typically will need to **reconfigure your workload from scratch** every single time.

This feature of `pebble-ready` events make them especially suitable for a [holistic handling pattern](https://discourse.charmhub.io/t/deltas-vs-holistic-charming/11095).


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?
 
TBA 

<!--

<a href="#heading--relation-event-triggers"><h2 id="heading--relation-event-triggers">Relation event triggers</h2></a>

Relation events trigger as a response to changes in the juju model relation topology. When a new relation is created or removed, events are fired on all units of both involved applications.

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
As such, contrary to many other events, `-relation-changed` events are mostly triggered by charm code (and not by the cloud admin doing things on the juju model).

```
-->


(hook-endpoint-relation-broken)=
### `<endpoint>-relation-broken`

#### What triggers it?

- A non-peer relation being removed; 
- a unit participating in a non-peer relation being removed, even if the relation is otherwise still alive (through other units); or 
- an application involved in a non-peer relation being removed.

This hook is fired only once per unit per relation and is the exact inverse of `relation-created`. `relation-created` indicates that relation data can be accessed; `relation-broken` indicates that relation data can no longer be read-written.

The hook indicates that the relation under consideration is no longer valid, and that the charm’s software must be configured as though the relation had never existed. It will only be called after every hook bound to `<endpoint>-relation-departed` has been run. If a hook bound to this event is being executed, it is guaranteed that no remote units are currently known locally.


> It is important to note that **the `-broken` hook might run even if no other units have ever joined the relation**. This is not a bug: even if no remote units have ever joined, the fact of the unit’s participation can be detected in other hooks via the `relation-ids` tool, and the `-broken` hook needs to execute to allow the charm to clean up any optimistically-generated configuration.

> Also, it’s important to internalise the fact that there may be multiple relations in play with the same name, and that they’re independent: one `-broken` hook does not mean that *every* such relation is broken.

> For a peer relation, `<peer relation name>-relation-broken` will never fire, not even during the teardown phase.


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-endpoint-relation-changed)=
### `<endpoint>-relation-changed`


#### What triggers it?

The `relation-changed` hook for a given unit always runs once immediately following the `relation-joined` hook for that unit, and subsequently whenever the related unit changes its settings (by calling `relation-set` and exiting without error). Note that immediately only applies within the context of this particular runtime relation -- that is, when `foo-relation-joined` is run for unit `bar/99` in relation id `foo:123`, the only guarantee is that/ the next hook to be run *in relation id `foo:123`* will be `foo-relation-changed` for `bar/99`. Unit hooks may intervene, as may hooks for other relations, and even for other foo relations.

`relation-changed` is emitted when another unit involved in the relation (from either side) touches the relation data. Relation data is *the* way for charms to share non-sensitive information (for sensitive information, `juju secrets` are on their way in juju 3).

> For centralized data -- for example, a single password or api token that one application generates to share with another application, we suggest that charm authors use the application data, rather than individual unit data. This data can only be written to by the application leader, and each remote unit related to that application will receive a single `relation-changed` event when it changes.

Hooks bound to this event should be the only ones that rely on remote relation settings. They should not error if the settings are incomplete, since it can be guaranteed that when the remote unit or application changes its settings again, this event will fire once more.

Charm authors should expect this event to fire many times during an application's life cycle. Units in an application are able to update relation data as needed, and a `relation-changed` event will fire every time the data in a relation changes. Since relation data can be updated on a per unit bases, a unit may receive multiple `relation-changed` events if it is related to multiple units in an application and all those units update their relation data. 

This event is guaranteed to follow immediately after each [`relation-joined`](https://discourse.charmhub.io/t/relation-name-relation-joined-event/6478). So all `juju` commands that trigger `relation-joined` will also cause `relation-changed` to be fired. So typical scenarios include: 

|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Add an integration   | `juju integrate foo:a bar:b` | (all `foo` and `bar` units) <br> `*-relation-created -> *-relation-joined -> *-relation-changed`|

Additionally, a unit will receive a `relation-changed` event every time another unit involved in the relation changes the relation data. Suppose `foo` receives an event, and while handling it the following block executes:

```python
# in charm `foo`
relation.data[self.unit]['foo'] = 'bar'  # set unit databag
if self.unit.is_leader():
    relation.data[self.app]['foo'] = 'baz'  # set app databag
```

When the hook returns, `bar` will receive a `relation-changed` event.

```{note}
 
Note that units only receive `relation-changed` events for **other** units' changes. This can matter in a peer relation, where the application leader will not receive a `relation-changed` event for the changes that it writes to the peer relation's application data bag. If all units, including the leader, need to react to a change in that application data, charm authors may include an inline `.emit()` for the `<name>_relation_changed` event on the leader. 

```


> **When is data synchronized?** <br>
Relation data is sent to the controller at the end of the hook's execution. If a charm author writes to local relation data multiple times during the a single hook run, the net change will be sent to the controller after the local code has finished executing. The controller inspects the data and determines whether the relation data has been changed. Related units then get the `relation-changed` event the next time they check in with the controller.



#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-endpoint-relation-created)=
### `<endpoint>-relation-created`

#### What triggers it?

`relation-created` is a "setup" event and, emitted when an application is related to another. Its purpose is to inform the newly related charms that they are entering the relation.

If Juju is aware of the existence of the relation "early enough", before the application has started (i.e. *before* the application has started, i.e., before the {ref}`start <event-start>` has run), this event will be fired as part of the setup phase. An important consequence of this fact is, that for all peer-type relations, since juju is aware of their existence from the start, those `relation-created`  events will always fire before `start`.

Similarly, if an application is being scaled up, the new unit will see `relation-created` events for all relations the application already has during the Setup phase.


|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
| Integrate  | `juju integrate foo bar` | (all foo & bar units): `*-relation-created --> *-relation-joined -> *-relation-changed` |
|  Scale up an integrated app | `juju add-unit -n1 foo` | (new foo unit): `install -> *-relation-created -> leader-settings-changed -> config-changed -> start` |


In the following scenario, one deploys two applications and relates them "very early on". For example, in a single command.
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Deploy and quickly integrate | `juju deploy foo; juju deploy bar; juju integrate foo bar` | (all units): same as previous case. |

Starting from when `*-relation-created` is received, relation data can be read-written by units, up until when the corresponding `*-relation-broken` is received.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA


(hook-endpoint-relation-departed)=
### `<endpoint>-relation-departed`

#### What triggers it?

<relation name>-relation-departed; emitted when a unit departs from an existing relation.

The "relation-departed" hook for a given unit always runs once when a related unit is no longer related. After the "relation-departed" hook has run, no further notifications will be received from that unit; however, its settings will remain accessible via relation-get for the complete lifetime of the relation.


`relation-departed` is a "teardown" event, emitted when a remote unit departs a relation. 
This event is the exact inverse of `relation-joined`.


`*-relation-broken` events are emitted on a unit when a related application is scaled down. Suppose you have two related applications, `foo` and `bar`.
If you scale down `bar`, all `foo` units will receive a `*-relation-departed` event. The departing unit will receive a `*-relation-broken` event as part of its {ref}`teardown sequence <charm-lifecycle>`.
Also removing a relation altogether will trigger `*-relation-departed` events (followed by `*-relation-broken`) on all involved units.

|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Unit removal   | `juju remove-unit --num-units 1 bar` | (foo/0): `*-relation-departed` <br> (bar/0): `*-relation-broken -> stop -> ...` |
|  Integration removal   | `juju remove-relation foo bar` | (all units): `*-relation-departed -> *-relation-broken` |

Of course, removing the application altogether, instead of a single unit, will have a similar effect and also trigger these events.

Both relation-departed and relation-broken will always fire, regardless of how the relation is terminated.

```{note}

For a peer relation, the relation itself will never be destroyed until the application is removed and no units remain, at which point there won't be anything to call the relation-broken hook on anyway.

```

Note that on relation removal (`juju remove-relation`); only when all `-departed` hooks for such a relation and all callback methods bound to this event have been run for such a relation, the unit agent will fire `relation-broken`.

The `relation-departed` event is seen both by the leaving unit(s) and the remaining unit(s):

- For remaining units (those which have joined and not yet departed), this event is emitted once for each departing unit and in no particular order. At the point when a remaining unit receives a `relation-departed`, it's perfectly probable (although not guaranteed) that the system running that unit has already shut down.
- For departing units, this event is emitted once for each remaining unit.


A unit's relation settings persist beyond its own departure from the relation; the final unit to depart a relation marked for termination is responsible for destroying the relation and all associated data.

`relation-changed` ISN'T fired for removed relations.
If you want to know when to remove a unit from your data, that would be relation-departed.

> During a `relation-departed` hook, relation settings can still be read (with relation-get) and a relation can even still be set (with relation-set), by  explicitly providing the relation ID. All units will still be able to see all other units, and any unit can call relation-set to update their own published set of data on the relation. However, data updated by the departing unit will not be published to the remaining units. This is true even if there are no units on the other side of the relation to be notified of the change.

If any affected unit publishes new data on the relation during the relation-departed hooks, the new data will NOT be see by the departing unit (it will NOT receive a relation-changed; only the remaining units will).


```{note}

(juju internals) When a unit's own participation in a relation is known to be ending, the unit agent continues to uphold the guaranteed event ordering, but within those constraints, it will run the fewest possible hooks to notify the charm of the departure of each individual remote unit.

```


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA


(hook-relation-joined)=
### `<endpoint>-relation-joined`

#### What triggers it?

<relation name>-relation-joined; emitted when a new unit joins in an existing relation.

The "relation-joined" hook always runs once when a related unit is first seen.


`relation-joined` is emitted when a unit joins in an existing relation. The unit will be a local one in the case of peer relations, a remote one otherwise.

By the time this event is emitted, the only available data concerning the relation is
 - the name of the joining unit.
 - the `private-address` of the joining unit.

In other words, when this event is emitted the remote unit has not yet had an opportunity to write any data to the relation databag. For that, you're going to have to wait for the first {ref}``relation-changed` <event-relation-name-relation-changed>` event.



From the perspective of an application called `foo`, which can relate to an application called `bar`:

|   Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju integrate foo bar` |  `*-relation-created -> *-relation-joined -> *-relation-changed` |
|  Create unit   | `juju add-unit bar -n 1`  |  `*-relation-joined -> *-relation-changed`|


```{note}

For a peer relation, `<peer relation name>-relation-joined` will be received by peers some time after a new peer unit appears. (And during its setup, that new unit will receive a  `<peer relation name>-relation-created`).

```

```{note}

For a peer relation, `<peer relation name>-relation-joined` will only be emitted if the scale is larger than 1. In other words, applications with a scale of 1 do not see peer relation joined/departed events.
**If you are using peer data as a means for persistent storage, then use peer `relation-created` instead**.

```

`relation-joined` can fire multiple times per relation, as multiple units can join, and is the exact inverse of `relation-departed`.
That means that if you consider the full lifecycle of an application, a unit, or a model, the net difference of the number of `*-relation-joined` events and the number of `*-relation-departed` events will be zero.



#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA


(hook-install)=
### `install`

#### What triggers it?

The `install` hook always runs once, and only once, before any other hook. 

fired when juju is done provisioning the unit.

The `install` event is emitted once per unit at the beginning of a charm's lifecycle. Associated callbacks should be used to perform one-time initial setup operations and prepare the unit to execute the application. Depending on the charm, this may include installing packages, configuring the underlying machine or provisioning cloud-specific resources.

Therefore, ways to cause `install` to occur are:
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo`<br>`juju add-unit foo`  | `install -> config-changed`|


> Note: 
> - Typically, operations performed on `install` should also be considered for [`upgrade-charm`](https://discourse.charmhub.io/t/upgrade-charm-event/6485).
> - In some cases, [`config-changed`](https://discourse.charmhub.io/t/config-changed-event/6465) can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.


The `install` event is emitted once per unit at the beginning of a charm's lifecycle. Associated callbacks should be used to perform one-time initial setup operations and prepare the unit to execute the application. Depending on the charm, this may include installing packages, configuring the underlying machine or provisioning cloud-specific resources.

Therefore, ways to cause `install` to occur are:
|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo`<br>`juju add-unit foo`  | `install -> config-changed`|


> Note: 
> - Typically, operations performed on `install` should also be considered for [`upgrade-charm`](https://discourse.charmhub.io/t/upgrade-charm-event/6485).
> - In some cases, [`config-changed`](https://discourse.charmhub.io/t/config-changed-event/6465) can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-leader-deposed)=
### `leader-deposed`

#### What triggers it?

TBA

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-leader-elected)=
### `leader-elected`

#### What triggers it?

The `leader-elected` event is emitted for a unit that is elected as leader. Together with `leader-settings-changed`, it is one of two "leadership events". A unit receiving this event can be guaranteed that it will have leadership for approximately 30 seconds (from the moment the event is received). After that time, juju *might* have elected a different leader. The same holds if the unit checks leadership by `Unit.is_leader()`: if the result is `True`, then the unit can be ensured that it has leadership for the next 30s.

> Leadership can change while a hook is running. (You could start a hook on unit/0 who is the leader, and while that hook is processing, you lose network connectivity for a long time [more than 30s], and then by the time the hook notices, juju has already moved on to another leader.) 

> Juju doesn't guarantee that a leader will see every event: if the leader unit is overloaded long enough for the lease to expire (>30s), then juju will elect a different leader. Events that fired in between would be received units that are not leader yet or not leader anymore.



- `leader-elected` is always emitted **after** peer-`relation-created` during the Startup phase. However, by the time `relation-created` runs, juju may already have a leader. This means that, in peer-relation-created handlers, it might already be the case that `self.unit.is_leader()` returns `True` even though the unit did not receive a leadership event yet. If the starting unit is *not* leader, it will receive a {ref}``leader-settings-changed` <event-leader-settings-changed>` instead.

|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Start new unit   | `juju deploy foo`<br>`juju add-unit foo`  | (new leader) `install -> (*peer)-relation-created -> leader-elected`|

-  During the Operation phase, leadership changes can in principle occur at any time, for example if the leader unit is unresponsive for some time. When a leader loses leadership it will only receive a `leader-settings-changed` event, just like all the other non-leader units. The new leader will receive `leader-elected`.

> It is not possible to select a specific unit and 'promote' that unit to leader, or 'demote' an existing leader unit. Juju has control over which unit will become leader after the current leader is gone. 

```{note}

However, you can cause leadership change by destroying the leader unit or killing the jujud-machine service in operator charms.

- non-k8s models: `juju remove-unit <leader_unit>`
- operator charms: `juju ssh -m <id> -- systemctl stop jujud-machine-<id>`
- sidecar charms: ssh into the charm container, source the `/etc/profile.d/juju-introspection.sh` script, and then get access to a few cli tools, including `juju_stop_unit`.

That will cause the lease to expire within 60s, and another unit of the same application will be elected leader and receive `leader-elected`. 

```

- If the leader unit is removed, then one of the remaining units will be elected as leader and see the `leader-elected` event; all the other remaining units will see `leader-settings-changed`. If the leader unit was not removed, no leadership events will be fired on any units.

> Note that unless there's only one unit left, it is impossible to predict or control which one of the remaining units will be elected as the new leader.

|   Scenario  | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Current leader loses leadership   | `juju remove-unit foo`  | (new leader): `leader-elected` <br> (all other foo units): `leader-settings-changed`|




#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?

The leader unit, once Juju elects one.


(hook-leader-settings-changed)=
### `leader-settings-changed`

#### What triggers it?

The `leader-settings-changed` event is emitted when a leadership change occurs, all units that are not the new leader will receive the event. Also, this event is emitted if changes have been made to leader settings.

During startup sequence, for all non-leader units:

|   Scenario   | Example Command  | Resulting Events |
| :-------: | -------------------------- | ------------------------------------ |
|  Create unit   | `juju deploy foo -n 2`  | `install -> leader-settings-changed -> config-changed -> start` (non-leader)|


If the leader unit is rescheduled, or removed entirely. When the new leader is elected:

|  Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Removal of leader   | `juju remove-unit foo/0` (foo/0 being leader)  | `leader-settings-changed` (for all non leaders) |

> Since this event needs leadership changes to trigger, check out {ref}`triggers for `leader-elected` <6471md>` as the same situations apply for `leader-settings-changed`.


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

All follower units, when a new leader is chosen.

(hook-post-series-upgrade)=
### `post-series-upgrade`
> Removed in Juju 4.

#### What triggers it?

Fired after the series upgrade has taken place.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-pre-series-upgrade)=
### `pre-series-upgrade`
> Removed in Juju 4.


#### What triggers it?

This event is triggered when an operator runs `juju upgrade-series <machine> prepare ...` from the command line.    Read [here](https://juju.is/docs/olm/upgrade-a-machines-series#heading--upgrading-a-machines-series) to learn more about the series upgrade process.  This event hook allows charm units on the machine being upgraded to do any necessary tasks prior to the upgrade process beginning (which may involve e.g. being rebooted, etc.). 

|  Scenario | Example command | Resulting events |
|:-:|-|-|
|[Upgrade series](https://juju.is/docs/olm/upgrade-a-machines-series#heading--upgrading-a-machines-series) | `juju upgrade-series <machine> prepare`| `pre-series-upgrade` -> (events on pause until upgrade completes) |

 Notably, after this event fires and before the [post-series-upgrade event](https://discourse.charmhub.io/t/post-series-upgrade-event/6472) fires, juju will pause events and changes for all units on the machine being upgraded.  There will be no config-changed, update-status, etc. events to interrupt the upgrade process until after the upgrade process is completed via `juju upgrade-series <machine> complete`.



```{caution}
 Leadership is pinned during the series upgrade process.  Even if the current leader dies or is removed, re-election will not occur for applications on the upgrading machine until the series upgrade operation completes.
```


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it? 

TBA

(hook-remove)=
### `remove`

#### What triggers it?

The `remove` event is emitted only once per unit: when the Juju controller is ready to remove the unit completely. The `stop` event is emitted prior to this, and all necessary steps for handling removal should be handled there.

On Kubernetes charms, the `remove` event will occur on pod churn, when the unit dies. On machine charms, the stop event will be fired when a unit is put down.

|   Scenario   | Example Command                          | Resulting Events                     |
| :-------: | -------------------------- | ------------------------------------ |
|  Remove unit   | `juju remove-unit foo/0` (on machine) or <br> `juju remove-unit --num-units 1 foo` (on k8s)  | `stop -> [relation/storage teardown] -> remove` |

Of course, removing an application altogether will result in these events firing on all units.

If the unit has any relations active or any storage attached at the time the removal occurs, these will be cleaned up (in no specific order) between `stop` and `remove`. This means the unit will receive `stop -> (*-relation-broken | *-storage-detaching) -> remove`.

The `remove` event is the last event a unit will ever see before going down, right after [`stop`](https://discourse.charmhub.io/t/6483). It is exclusively fired when the unit is in the [Teardown phase](https://discourse.charmhub.io/t/5938).

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA


(hook-secret-changed)=
### `secret-changed`

#### What triggers it?

A secret owner publishing a new secret revision.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

All units observing a secret.

```{note}
Upon receiving that event (or at any time after that) an observer can choose to:

 - Start tracking the latest revision ("refresh")
 - Inspect the latest revision values, without tracking it just yet ("peek")

Once all observers have stopped tracking a specific outdated revision, the owner will receive a `secret-remove` hook to be notified of that fact, and can then remove that revision.
```

(hook-secret-expired)=
### `secret-expired`
> Currently supported only for charm secrets.
> 
> Added in Juju 3.0.2

#### What triggers it?

For a secret set up with an expiry date, the fact of the secret’s expiration time elapsing. 

#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?

The secret owner.

(hook-secret-remove)=
### `secret-remove`
> Currently supported only for charm secrets.

#### Who gets it?

The secret owner.


#### What triggers it?

A secret revision no longer having any observers and thus being safe to remove.

This situation can occur when:

- All observers tracking a now-outdated revision have updated to tracking a newer one, so the old revision can be removed.
- No observer is tracking an intermediate revision, and a newer one has already been created. So there is a orphaned revision which no observer will ever be able to peek or update to, because there is already a newer one the observer would get instead.

In short, the purpose of this event is to notify the owner of a secret that a specific revision of it is safe to remove: no charm is presently observing it or ever will be able to in the future.


#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA


(hook-secret-rotate)=
### `secret-rotate`
> Currently supported only for charm secrets.
> 
> Added in Juju 3.0.2

#### What triggers it?

For a secret with a rotation policy, the secret's rotation policy elapsing -- the hook keeps firing until the secret has been rotated.

#### Which hooks can be guaranteed to have fired before it, if any?

#### Which environment variables is it executed with? 

#### Who gets it?

The secret owner.

(hook-start)=
### `start`

#### What triggers it?

A unit's initialization being complete.

This can occur:

- when the unit is being created
- (Kubernetes:) on pod churn
- (Kubernetes:) on cluster reboot
- on charm upgrades

For Kubernetes charms, this occurs on pod churn as well.

```{note}
Callback methods bound to the event should be used to ensure that the charm’s software is in a running state. Note that the charm’s software should be configured to persist in a started state without further intervention from Juju or an administrator.
```

```{note}

In kubernetes sidecar charms, Juju provides no ordering guarantees regarding `start` and `*-pebble-ready`.

```

#### Which hooks can be guaranteed to have fired before it, if any?

The `config-changed` hook. (The `start` hook is fired immediately after.)

#### Which environment variables is it executed with? 

#### Who gets it?

Any unit.

(hook-stop)=
### `stop`

#### What triggers it?

The Juju controller being ready to destroy the unit. 

This can occur:

- when the unit is being removed (whether explicitly or through the application as a whole being removed)
- (Kubernetes:) on pod churn


The `stop` hook is the  one-before-last  hook the unit will receive before being destroyed (the last one being `remove`).

```{note}
 When handling the `stop` event, charms should gracefully terminate all services for the supported application and update any relevant cluster/leader information to remove or update any data relating to the current unit. Additionally, the charm should ensure that the software will not automatically start again on reboot.

```

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 
TBA

#### Who gets it?

Any unit.



(hook-storage-storage-attached)=
### `<storage>-storage-attached`

#### What triggers it?

A storage volume having been attached to the charm's host machine or container and being ready to be interacted with.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

(hook-storage-storage-detaching)=
#### `<storage>-storage-detaching`

#### What triggers it?

A request to detach storage having been processed.

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA


(hook-update-status)=
### `update-status`

#### What triggers it?

Nothing in particular -- this hooks is fired automatically by Juju at regular intervals (default: 5m; can be changed, e.g., `juju model-config update-status-hook-interval=1m`).

```{note}
This event can be used to monitor the health of deployed charms and determine the status of long running tasks (such as package installation), updating the status message reported to Juju accordingly.

Historically, this hook was intended to allow authors to run code that gets the “health” of the application. However, health checks can also be specified via {ref}`pebble`.

Since the update-status interval is model-wide (not per application) and is set by the user (for example, it can be set to once per hour), charms should not rely on it for critical operations.

In integration tests, unless specifically testing the update-status hook, you may want to "disable" it so it doesn't interfere with the test. This can be achieved by setting the interval to e.g. 1h at the beginning of the test.

```

#### Which hooks can be guaranteed to have fired before it, if any?

As it is triggered periodically, the `update-status`  can happen in-between any other charm events.

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

TBA

By default, the `update-status` event is triggered by the Juju controller at 5-minute intervals.


(hook-upgrade-charm)=
### `upgrade-charm`

#### What triggers it?


The `upgrade-charm` hook always runs once immediately after the charm directory contents have been changed by an unforced charm upgrade operation, and *may* do so after a forced upgrade; but will *not* be run after a forced upgrade from an existing error state. (Consequently, neither will the `config-changed` hook that would ordinarily follow the `upgrade-charm`.


The event is emitted after the new charm code has been unpacked - therefore this event is handled by the callback method bound to the event in the new codebase.

The associated callback should be used to reconcile the current state written by an older version into whatever form is required by the current charm version. An example of a reconciliation that needs to take place is to migrate an old relation data schema to a new one.

```{important}

- Typically, operations performed on `upgrade-charm` should also be considered for [`install`](https://discourse.charmhub.io/t/install-event/6469).
- In some cases, [`config-changed`](https://discourse.charmhub.io/t/config-changed-event/6465) can be used instead of `install` and `upgrade-charm` because it is guaranteed to fire after both.
- Note that you cannot upgrade a Charmhub charm to the same version. However, upgrading a local charm from path works (and goes through the entire upgrade sequence) even if the charm is exactly the same.

```

|  Scenario | Example command | Resulting events |
|:-:|-|-|
|Upgrade Kubernetes charm | `juju refresh` | `stop` (old charm) -> `upgrade-charm` (new charm) -> `config-changed` -> `leader-settings-changed` (if the unit is not the leader) -> `start` -> `*-pebble-ready`|
|Upgrade machine charm | `juju refresh` | `upgrade-charm` -> `config-changed` -> `leader-settings-changed` (if the unit is not the leader) -> `start`|
|Attach resource | `juju attach-resource foo bar=baz` | (same as upgrade) |

```{important}

An upgrade does NOT trigger any relation hooks (unless relation data is intentionally modified in one of the upgrade sequence hooks).
```

#### Which hooks can be guaranteed to have fired before it, if any?

TBA

#### Which environment variables is it executed with? 

TBA

#### Who gets it?

Any unit.

