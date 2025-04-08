# Entity lifecycle

In Juju, certain fundamental state entities have "lifecycles". These entities
are:

  * Machines
  * Units
  * Applications
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

(entity-creation)=
## Entity creation

This section describes the circumstances in which fundamental state entities
are created, from the perspective of the CLI.

### `juju deploy`

The `juju deploy` command always creates applications, may create relations, and
may also create units and machines.

  * New applications can always be added.
  * If the created application's charm defines any peer relations, a (runtime) peer
    relation will be created for each. BUG: this is not done in the same
    transaction as application creation; a connection failure at the wrong time
    will create a broken and unfixable application (because peer relations cannot
    be manipulated via the CLI).
  * If the created application's charm is not subordinate, some number of units will
    be created; this number is controlled via the "--num-units" parameter which
    defaults to 1.
  * If units were created, machines may also be created, as below.


### `juju add-unit`

The `juju add-unit` command applies only to principal applications. It always
creates units, and may create machines. Different providers assign units to
machines in different ways, and so machine creation can vary: for example, the
ec2 provider creates a new machine for each unit that cannot be placed on an
existing machine without assigned units.

  * New units can only be added to Alive applications.


### `juju integrate`

The `juju integrate` command creates relations, and may -- if the relation
has container scope, by virtue of one or more endpoints having container scope
-- indirectly cause the creation of subordinate units. Subordinate units are in
fact created by principal unit agents, at the point when they enter scope of a
container-scoped relation and find that no suitable subordinate already exists.

  * New relations can only be added between Alive applications.
  * New subordinate units will only be added as a consequence of an Alive
    principal unit's participation in an Alive relation (implying an Alive
    subordinate application).

## Entity death and destruction

This section describes in detail the operations associated with the destruction
and removal of the fundamental state entities, and what agents are responsible
for those operations.

Each entity has an associated remove-* command. The precise implications of
removal differ by entity, but there are common features:

  * Only Alive entities can be removed; if removal is already in progress,
    as evidenced by an entity not being Alive, its "removal" is a no-op.
  * Entities might be removed immediately when they are destroyed, but this is not
    guaranteed.
  * If an entity is not removed immediately when it is destroyed, its eventual
    removal is very likely; but it is not currently guaranteed, for the
    following reasons:
      * Hardware failure, even when detected and corrected by a Provisioner, can
        lead to unremovable relations, because the redeployed unit doesn't know
        what relations it's in. This would be fixable by making the unit agent
        always leave the scope of relations when they're detected; or, probably
        better, by using actual remote scope membership state to track relation
        membership (rather than using the existence of a local directory, whose
        true intent is to track the membership of *other* units, as a proxy).
        This is actually a pretty serious BUG and should be addressed soon;
        neither proposed solution is very challenging.
      * Undetected hardware failure is annoying, and can block progress at any
        time, but can be observed via additional monitoring and resolved via out-
        of-band termination of borked resources, which should be sufficient to
        get the system moving again (assuming the above bug is fixed).
      * Unknown problems in juju, in which agents fail to fulfil the duties laid
        out in this document, could block progress at any time. Assuming a
        version of the agent code which does not exhibit the problem exists, it
        should always be possible to work around this situation by upgrading the
        agent; and, if that fails, by terminating the underlying provider
        resources out-of-band, as above, and waiting for the new agent version
        to be deployed on a fresh system (with the same caveat as above).
      * In light of the preceding two points, we don't *have* to implement
        "--force" options for `juju remove-machine` and `juju remove-unit`.
        This is good, because it will be tricky to implement them well.

In general, the user can just forget about entities once she's destroyed them;
the only caveat is that she may not create new applications with the same name, or
new relations identical to the destroyed ones, until those entities have
finally been removed.

In rough order of complexity, here's what happens when each entity kind is
destroyed. Note that in general the appropriate action is contingent on
mutable remote state, and many operations must be expressed as a transaction
involving several documents: the state API must be prepared to handle aborted
transactions and either diagnose definite failure or retry until the operation
succeeds (or, perhaps, finally error out pleading excessive contention).


### `juju remove-machine`


Removing a machine involves a single transaction defined as follows:

  * If the machine is not Alive, abort without error.
  * If the machine is the last one with JobManageModel, or has any assigned
    units, abort with an appropriate error.
  * Set the machine to Dying.

When a machine becomes Dying, the following operation occurs:

  * The machine's agent sets the machine to Dead.

When a machine becomes Dead, the following operations occur:

  * The machine's agent terminates itself and refuses to run again.
  * A Provisioner (a task running in some other machine agent) observes the
    death, decommissions the machine's resources, and removes the machine.

Removing a machine involves a single transaction defined as follows:

  * If the machine is not Dead, abort with an appropriate error.
  * Delete the machine document.


### `juju remove-unit`

Removing a unit involves a single transaction defined as follows:

  * If the unit is not Alive, abort without error.
  * Set the unit to Dying.

When a unit becomes Dying, the following operations occur:

  * The unit's agent leaves the scopes of all its relations. Note that this is
    a potentially complex sequence of operations and may take some time; in
    particular, any hooks that fail while the unit is leaving relations and
    stopping the charm will suspend this sequence until resolved (just like
    when the unit is Alive).
  * The unit's agent then sets the unit to Dead.

When a unit becomes Dead, the following operations occur:

  * The unit's agent terminates itself and refuses to run again.
  * The agent of the entity that deployed the unit (that is: a machine agent,
    for a principal unit; or, for a subordinate unit, the agent of a principal
    unit) observes the death, recalls the unit, and removes it.

Removing a unit involves a single transaction, defined as follows:

  * If the unit is a principal unit, unassign the unit from its machine.
  * If the unit is a subordinate unit, unassign it from its principal unit.
  * Delete the unit document.
  * If its application is Alive, or has at least two units, or is in at least
    one relation, decrement the application's unit count; otherwise remove the
    application.


### `juju remove-relation`

Removing a relation involves a single transaction defined as follows:

  * If the relation is not Alive, abort without error.
  * If any unit is in scope, set the relation to Dying.
  * Otherwise:
      * If the relation destruction is a direct user request, decrement the
        relation counts of both applications.
      * If the relation destruction is an immediate consequence of application
        destruction, decrement the reference count of the counterpart application
        alone. (This is because the application destruction logic is responsible
        for the relation count of the application being destroyed.)
      * Delete the relation document.
      * Mark the relation's unit settings documents for future cleanup.
          * This is done by creating a single document for the attention of
            some other part of the system (BUG: which doesn't exist), that is
            then responsible for mass-deleting the (potentially large number
            of) settings documents. This completely bypasses the mgo/txn
            mechanism, but we don't care because those documents are guaranteed
            to be unreferenced and unwatched, by virtue of the relation's prior
            removal.

When a relation is set to Dying, the following operations occur:

  * Every unit agent whose unit has entered the scope of that relation
    observes the change and causes its unit to leave scope.
  * If the relation has container scope, and no other container-scoped relation
    between its applications is Alive, the unit agents of the subordinate units in
    the relation will observe the change and destroy their units.

The Dying relation's document is finally removed in the same transaction in
which the last unit leaves its scope. Because this situation involves the
relation already being Dying, its applications may also be Dying, and so the
operations involved are subtly different to those taken above (when we know
for sure that the relation -- and hence both applications -- are still Alive).

  * Here, "the application" refers to the application of the unit departing scope, and
    "the counterpart application" refers to the other application in the relation.
  * Decrement the relation count of the unit's application (we know that application
    is not ready to be removed, because its unit is responsible for this
    transaction and the application clearly therefore has a unit count greater
    than zero).
  * Delete the relation document.
  * Mark the relation's unit settings documents for future cleanup.
  * If the counterpart application (the one that is not the unit's application) is
    Alive, or has at least one unit, or is in at least two relations, decrement
    its relation count; otherwise remove the counterpart application.


### `juju remove-application`

Removing an application involves a single transaction defined as follows:

  * If the application is not alive, abort without error.
  * If the application is in any relations, do the following for each one:
      * If the relation is already Dying, skip it.
      * If the relation is Alive, destroy the relation without modifying the
        application's relation count. If the relation's destruction implies its
        removal, increment a local removed-relations counter instead.
  * If the application's unit count is greater than 0, or if the value of the
    aforementioned removal counter is less than the application's relation count,
    we know that some entity will still hold a reference to the application after
    the transaction completes, so we set the application to Dying and decrement
    its relation count by the value of the removal counter.
  * Otherwise, remove the application immediately, because we know that no
    reference to the application will survive the transaction.

When an application becomes Dying, the following operations occur:

  * Every unit agent of the application observes the change and destroys its unit.

The Dying application's document is finally removed in the same transaction that
removes the last entity referencing that application. This could be either the
removal of the last unit in the application, or the removal of the last relation
the application is in, as described above. To remove an application, the following
operations must occur in a single transaction:

  * Remove the application document.
  * Remove the application's settings document.




## Machines


  * Like everything else, a machine starts out Alive. `juju bootstrap` aside,
    the user interface does not allow for direct creation of machines, but
    `juju deploy` and `juju add-unit` may create machines as a consequence of
    unit creation.
  * If a machine has the JobManageModel job, it cannot become Dying or Dead.
    Other jobs do not affect the lifecycle directly.
  * If a machine has the JobHostUnits job, principal units can be assigned to it
    while it is Alive.
  * While principal units are assigned to a machine, its lifecycle cannot change
    and `juju remove-machine` will fail.
  * When no principal units are assigned, `juju remove-machine` will set the
    machine to Dying. (Future plans: allow a machine to become Dying when it
    has principal units, so long as they are not Alive. For now it's extra
    complexity with little direct benefit.)
  * When a machine has containers, `juju remove-machine` will fail, unless force
    is used.  However `juju destroy-controller` or `juju destroy-model` allows a
    machine to move to dying with containers.
  * Once a machine has been set to Dying, the corresponding Machine Agent (MA)
    is responsible for setting it to Dead. A dying machine cannot transition to
    dead if there are containers. (Future plans: when Dying units are
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

## Units

  * A principal unit can be created directly with `juju deploy` or
    `juju add-unit`.
  * While a principal unit is Alive, it can be assigned to a machine.
  * While a principal unit is Alive, it can enter the scopes of Alive
    relations, which may cause the creation of subordinate units; so,
    indirectly, `juju integrate` can also cause the creation of units.
  * A unit can become Dying at any time, but may not become Dead while any unit
    subordinate to it exists, or while the unit is in scope for any relation.
  * A principal unit can become Dying in one of two ways:
      * `juju remove-unit` (This doesn't work on subordinates; see below.)
      * `juju remove-application` (This does work on subordinates, but happens
        indirectly in either case: the Unit Agents (UAs) for each unit of an
        application set their corresponding units to Dying when they detect their
        application Dying; this is because we try to assume 100k-scale and we can't
        use mgo/txn to do a bulk update of 100k units: that makes for a txn
        with at least 100k operations, and that's just crazy.)
  * A subordinate must also become Dying when either:
      * its principal becomes Dying, via `juju remove-unit`; or
      * the last Alive relation between its application and its principal's
        application is no longer Alive. This may come about via `juju remove-relation`.
  * When any unit is Dying, its UA is responsible for removing impediments to
    the unit becoming Dead, and then making it so. To do so, the UA must:
      * Depart from all its relations in an orderly fashion.
      * Wait for all its subordinates to become Dead, and remove them from state.
      * Set its unit to Dead.
  * As just noted, when a subordinate unit is Dead, it is removed from state by
    its principal's UA; the relationship is the same as that of a principal unit
    to its assigned machine agent, and of a machine to the JobManageModel
    machine agent.

## Applications

  * Applications are created with `juju deploy`. Applications with duplicate names
    are not allowed (units and machine with duplicate names are not possible:
    their identifiers are assigned by juju).
  * Unlike units and machines, applications have no corresponding agent.
  * In addition, applications become Dead and are removed from the database in a
    single atomic operation.
  * When an application is Alive, units may be added to it, and relations can be
    added using the application's endpoints.
  * An applications can be destroyed at any time, via `juju remove-application`.
    This causes all the units to become Dying, as discussed above, and will also
    cause all relations in which the application is participating to become Dying
    or be removed.
  * If a removed application has no units, and all its relations are eligible
    for immediate removal, then the application will also be removed immediately
    rather than being set to Dying.
  * If no associated relations exist, the application is removed by the MA which
    removes the last unit of that application from state.
  * If no units of the application remain, but its relations still exist, the
    responsibility for removing the application falls to the last UA to leave scope
    for that relation. (Yes, this is a UA for a unit of a totally different
    application.)

## Relations

  * A relation is created with `juju integrate`. No two relations with the
    same canonical name can exist. (The canonical relation name form is
    "<requirer-endpoint> <provider-endpoint>", where each endpoint takes the
    form "<application-name>:<charm-relation-name>".)
      * Thanks to convention, the above is not strictly true: it is possible
        for a subordinate charm to require a container-scoped "juju-info"
        relation. These restrictions mean that the name can never cause
        actual ambiguity; nonetheless, support should be phased out smoothly
        (see lp:1100076).
  * A relation, like an application, has no corresponding agent; and becomes Dead
    and is removed from the database in a single operation.
  * Similarly to an application, a relation cannot be created while an identical
    relation exists in state (in which identity is determined by equality of
    canonical relation name -- a sequence of endpoint pairs sorted by role).
  * While a relation is Alive, units of applications in that relation can enter its
    scope; that is, the UAs for those units can signal to the system that they
    are participating in the relation.
  * A relation can be destroyed with either `juju remove-relation` or
    `juju remove-application`.
  * When a relation is destroyed with no units in scope, it will immediately
    become Dead and be removed from state, rather than being set to Dying.
  * When a relation becomes Dying, the UAs of units that have entered its scope
    are responsible for cleanly departing the relation by running hooks and then
    leaving relation scope (signalling that they are no longer participating).
  * When the last unit leaves the scope of a Dying relation, it must remove the
    relation from state.
  * As noted above, the Dying relation may be the only thing keeping a Dying
    application (different to that of the acting UA) from removal; so, relation
    removal may also imply application removal.

## References

OK, that was a bit of a hail of bullets, and the motivations for the above are
perhaps not always clear. To consider it from another angle:

  * Subordinate units reference principal units.
  * Principal units reference machines.
  * All units reference their applications.
  * All units reference the relations whose scopes they have joined.
  * All relations reference the applications they are part of.

In every case above, where X references Y, the life state of an X may be
sufficient to prevent a change in the life state of a Y; and, conversely, a
life change in an X may be sufficient to cause a life change in a Y. (In only
one case does the reverse hold -- that is, setting an application or relation to
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
    |   +----------+       +-------------+
    |   | relation |------>| application |
    |   +----------+       +-------------+
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
  * destroying a application destroys all its units and relations
  * destroying a container relation destroys all subordinates in the relation
  * (destroying a global relation destroys nothing else)

...and it takes a combination of these viewpoints to understand the detailed
interactions laid out above.

## Agents

It may also be instructive to consider the responsibilities of the unit and
machine agents. The unit agent is responsible for:

  * detecting Alive relations incorporating its application and entering their
    scopes (if a principal, this may involve creating subordinates).
  * detecting Dying relations whose scope it has entered and leaving their
    scope (this involves removing any relations or applications that thereby
    become unreferenced).
  * detecting undeployed Alive subordinates and deploying them.
  * detecting undeployed non-Alive subordinates and removing them (this raises
    similar questions to those alluded to above re Dying units on Dying machines:
    but, without persistent storage, there's no point deploying a Dying unit just
    to wait for its agent to set itself to Dead).
  * detecting deployed Dead subordinates, recalling them, and removing them.
  * detecting its application's Dying state, and setting its own Dying state.
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
    (recall that unit removal may imply application removal).
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

## Implementation

All state change operations are mediated by the mgo/txn package, which provides
multi-document transactions aginst MongoDB. This allows us to enforce the many
conditions described above without experiencing races, so long as we are mindful
when implementing them.

Lifecycle support is not complete: relation lifecycles are, mostly, as are
large parts of the unit and machine agent; but substantial parts of the
machine, unit and application entity implementation still lack sophistication.
This situation is being actively addressed.

Beyond the plans detailed above, it is important to note that an agent that is
failing to meet its responsibilities can have a somewhat distressing impact on
the rest of the system. To counteract this, we have implemented a --force
flag to remove-unit and remove-machine that forcibly sets an entity to
Dead while maintaining consistency and sanity across all references.
