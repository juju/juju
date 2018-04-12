Lifecycles
==========

In juju, certain fundamental state entities have "lifecycles". These entities
are:

  * Machines
  * Units
  * Services
  * Relations

...and there are only 3 possible states for the above things:

  * Alive (An entity is Alive when it is first created.)
  * Dying (An entity becomes Dying when the user indicates that it should be
    destroyed, and remains so while there are impediments to its removal.)
  * Dead (an entity becomes Dead when there are no further impediments to
    its removal; at this point it may be removed from the database at any time.
    Some entities may become Dead and are removed as a single operation, and
    are hence never directly observed to be "Dead", but should still be so
    considered.)

There are two fundamental truths in this system:

  * All such things start existence Alive.
  * No such thing can ever change to an earlier state.

Beyond the above rules, lifecycle shifts occur at different times for different
kinds of entities.

Machines
--------

  * Like everything else, a machine starts out Alive. `juju bootstrap` aside,
    the user interface does not allow for direct creation of machines, but
    `juju deploy` and `juju add-unit` may create machines as a consequence of
    unit creation.
  * If a machine has the JobManageModel job, it cannot become Dying or Dead.
    Other jobs do not affect the lifecycle directly.
  * If a machine has the JobHostUnits job, principal units can be assigned to it
    while it is Alive.
  * While principal units are assigned to a machine, its lifecycle cannot change
    and `juju destroy-machine` will fail.
  * When no principal units are assigned, `juju destroy-machine` will set the
    machine to Dying. (Future plans: allow a machine to become Dying when it
    has principal units, so long as they are not Alive. For now it's extra
    complexity with little direct benefit.)
  * Once a machine has been set to Dying, the corresponding Machine Agent (MA)
    is responsible for setting it to Dead. (Future plans: when Dying units are
    assigned, wait for them to become Dead and remove them completely before
    making the machine Dead; not an issue now because the machine can't yet
    become Dying with units assigned.)
  * Once a machine has been set to Dead, the agent for some other machine (with
    JobManageModel) will release the underlying instance back to the provider
    and remove the machine entity from state. (Future uncertainty: should the
    provisioner provision an instance for a Dying machine? At the moment, no,
    because a Dying machine can't have any units in the first place; in the
    future, er, maybe, because those Dying units may be attached to persistent
    storage and should thus be allowed to continue to shut down cleanly as they
    would usually do. Maybe.)

Units
-----

  * A principal unit can be created directly with `juju deploy` or
    `juju add-unit`.
  * While a principal unit is Alive, it can be assigned to a machine.
  * While a principal unit is Alive, it can enter the scopes of Alive
    relations, which may cause the creation of subordinate units; so,
    indirectly, `juju add-relation` can also cause the creation of units.
  * A unit can become Dying at any time, but may not become Dead while any unit
    subordinate to it exists, or while the unit is in scope for any relation.
  * A principal unit can become Dying in one of two ways:
      * `juju destroy-unit` (This doesn't work on subordinates; see below.)
      * `juju destroy-service` (This does work on subordinates, but happens
        indirectly in either case: the Unit Agents (UAs) for each unit of a
        service set their corresponding units to Dying when they detect their
        service Dying; this is because we try to assume 100k-scale and we can't
        use mgo/txn to do a bulk update of 100k units: that makes for a txn
        with at least 100k operations, and that's just crazy.)
  * A subordinate must also become Dying when either:
      * its principal becomes Dying, via `juju destroy-unit`; or
      * the last Alive relation between its service and its principal's service
        is no longer Alive. This may come about via `juju destroy-relation`.
  * When any unit is Dying, its UA is responsible for removing impediments to
    the unit becoming Dead, and then making it so. To do so, the UA must:
      * Depart from all its relations in an orderly fashion.
      * Wait for all its subordinates to become Dead, and remove them from state.
      * Set its unit to Dead.
  * As just noted, when a subordinate unit is Dead, it is removed from state by
    its principal's UA; the relationship is the same as that of a principal unit
    to its assigned machine agent, and of a machine to the JobManageModel
    machine agent.

Services
--------

  * Services are created with `juju deploy`. Services with duplicate names
    are not allowed (units and machine with duplicate names are not possible:
    their identifiers are assigned by juju).
  * Unlike units and machines, services have no corresponding agent.
  * In addition, services become Dead and are removed from the database in a
    single atomic operation.
  * When a service is Alive, units may be added to it, and relations can be
    added using the service's endpoints.
  * A service can be destroyed at any time, via `juju destroy-service`. This
    causes all the units to become Dying, as discussed above, and will also
    cause all relations in which the service is participating to become Dying
    or be removed.
  * If a destroyed service has no units, and all its relations are eligible
    for immediate removal, then the service will also be removed immediately
    rather than being set to Dying.
  * If no associated relations exist, the service is removed by the MA which
    removes the last unit of that service from state.
  * If no units of the service remain, but its relations still exist, the
    responsibility for removing the service falls to the last UA to leave scope
    for that relation. (Yes, this is a UA for a unit of a totally different
    service.)

Relations
---------

  * A relation is created with `juju add-relation`. No two relations with the
    same canonical name can exist. (The canonical relation name form is
    "<requirer-endpoint> <provider-endpoint>", where each endpoint takes the
    form "<application-name>:<charm-relation-name>".)
      * Thanks to convention, the above is not strictly true: it is possible
        for a subordinate charm to require a container-scoped "juju-info"
        relation. These restrictions mean that the name can never cause
        actual ambiguity; nonetheless, support should be phased out smoothly
        (see lp:1100076).
  * A relation, like a service, has no corresponding agent; and becomes Dead
    and is removed from the database in a single operation.
  * Similarly to a service, a relation cannot be created while an identical
    relation exists in state (in which identity is determined by equality of
    canonical relation name -- a sequence of endpoint pairs sorted by role).
  * While a relation is Alive, units of services in that relation can enter its
    scope; that is, the UAs for those units can signal to the system that they
    are participating in the relation.
  * A relation can be destroyed with either `juju destroy-relation` or
    `juju destroy-service`.
  * When a relation is destroyed with no units in scope, it will immediately
    become Dead and be removed from state, rather than being set to Dying.
  * When a relation becomes Dying, the UAs of units that have entered its scope
    are responsible for cleanly departing the relation by running hooks and then
    leaving relation scope (signalling that they are no longer participating).
  * When the last unit leaves the scope of a Dying relation, it must remove the
    relation from state.
  * As noted above, the Dying relation may be the only thing keeping a Dying
    service (different to that of the acting UA) from removal; so, relation
    removal may also imply service removal.

References
----------

OK, that was a bit of a hail of bullets, and the motivations for the above are
perhaps not always clear. To consider it from another angle:

  * Subordinate units reference principal units.
  * Principal units reference machines.
  * All units reference their services.
  * All units reference the relations whose scopes they have joined.
  * All relations reference the services they are part of.

In every case above, where X references Y, the life state of an X may be
sufficient to prevent a change in the life state of a Y; and, conversely, a
life change in an X may be sufficient to cause a life change in a Y. (In only
one case does the reverse hold -- that is, setting a service or relation to
Dying will cause appropriate units' agents to individually set their units to
Dying -- and this is just an implementation detail.)

The following scrawl may help you to visualize the references in play:

        +-----------+       +---------+
    +-->| principal |------>| machine |
    |   +-----------+       +---------+
    |      |     |
    |      |     +--------------+
    |      |                    |
    |      V                    V
    |   +----------+       +---------+
    |   | relation |------>| service |
    |   +----------+       +---------+
    |      A                    A
    |      |                    |
    |      |     +--------------+
    |      |     |
    |   +-------------+
    +---| subordinate |
        +-------------+

...but is important to remember that it's only one view of the relationships
involved, and that the user-centric view is quite different; from a user's
perspective the influences appear to travel in the opposite direction:

  * (destroying a machine "would" destroy its principals but that's disallowed)
  * destroying a principal destroys all its subordinates
  * (destroying a subordinate directly is impossible)
  * destroying a service destroys all its units and relations
  * destroying a container relation destroys all subordinates in the relation
  * (destroying a global relation destroys nothing else)

...and it takes a combination of these viewpoints to understand the detailed
interactions laid out above.

Agents
------

It may also be instructive to consider the responsibilities of the unit and
machine agents. The unit agent is responsible for:

  * detecting Alive relations incorporating its service and entering their
    scopes (if a principal, this may involve creating subordinates).
  * detecting Dying relations whose scope it has entered and leaving their
    scope (this involves removing any relations or services that thereby
    become unreferenced).
  * detecting undeployed Alive subordinates and deploying them.
  * detecting undeployed non-Alive subordinates and removing them (this raises
    similar questions to those alluded to above re Dying units on Dying machines:
    but, without persistent storage, there's no point deploying a Dying unit just
    to wait for its agent to set itself to Dead).
  * detecting deployed Dead subordinates, recalling them, and removing them.
  * detecting its service's Dying state, and setting its own Dying state.
  * if a subordinate, detecting that no relations with its principal are Alive,
    and setting its own Dying state.
  * detecting its own Dying state, and:
      * leaving all its relation scopes;
      * waiting for all its subordinates to be removed;
      * setting its own Dead state.

A machine agent's responsibilities are determined by its jobs. There are only
two jobs in existence at the moment; an MA whose machine has JobHostUnits is
responsible for:

  * detecting undeployed Alive principals assigned to it and deploying them.
  * detecting undeployed non-Alive principals assigned to it and removing them
    (recall that unit removal may imply service removal).
  * detecting deployed Dead principals assigned to it, recalling them, and
    removing them.
  * detecting deployed principals not assigned to it, and recalling them.
  * detecting its machine's Dying state, and setting it to Dead.

...while one whose machine has JobManageModel is responsible for:

  * detecting Alive machines without instance IDs and provisioning provider
    instances to run their agents.
  * detecting non-Alive machines without instance IDs and removing them.
  * detecting Dead machines with instance IDs, decommissioning the instance, and
    removing the machine.

Machines can in theory have multiple jobs, but in current practice do not.

Implementation
--------------

All state change operations are mediated by the mgo/txn package, which provides
multi-document transactions aginst MongoDB. This allows us to enforce the many
conditions described above without experiencing races, so long as we are mindful
when implementing them.

Lifecycle support is not complete: relation lifecycles are, mostly, as are
large parts of the unit and machine agent; but substantial parts of the
machine, unit and service entity implementation still lack sophistication.
This situation is being actively addressed.

Beyond the plans detailed above, it is important to note that an agent that is
failing to meet its responsibilities can have a somewhat distressing impact on
the rest of the system. To counteract this, we intend to implement a --force
flag to destroy-unit (and destroy-machine?) that forcibly sets an entity to
Dead while maintaining consistency and sanity across all references. The best
approach to this problem has yet to be agreed; we're not short of options, but
none are exceptionally compelling.
