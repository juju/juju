What We Run, and Why
====================

Expressed as compactly as possible, the Provisioner is responsible for making
sure that non-Dead machine entities in state have agents running on live
instances; and for making sure that Dead machines, and stray instances, are
removed and cleaned up.

However, the choice of exactly what we deploy involves some subtleties. At the
Provisioner level, it's simple: the series and the constraints we pass to the
Environ.StartInstance come from the machine entity. But how did they get there?

Series
------

Individual charms are released for different possible target series; juju
should guarantee that charms for series X are only ever run on series X.
Every service, unit, and machine has a series that's set at creation time and
subsequently immutable. Units take their series from their service, and can
only be assigned to machines with matching series.

Subordinate units cannot be assigned directly to machines; they are created
by their principals, on the same machine, in response to the creation of
subordinate relations. We therefore restrict subordinate relations such that
they can only be created between services with matching series.

Constraints
-----------

Constraints are stored for environments, services, units, and machines, but
unit constraints are not currently exposed because they're not needed outside
state, and are likely to just cause trouble and confusion if we expose them.

From the point of a user, there are environment constraints and service
constraints, and sensible manipulations of them lead to predictable unit
deployment decisions. The mechanism is as follows:

  * when a unit is added, the current environment and service constraints
    are collapsed into a single value and stored for the unit. (To be clear:
    at the moment the unit is created, the current service and environment
    constraints will be combined such that every constraint not set on the
    service is taken from the environment (or left unset, if not specified
    at all).
  * when a machine is being added in order to host a given unit, it copies
    its constraints directly from the unit.
  * when a machine is being added without a unit associated -- for example,
    when adding additional state servers -- it copies its constraints directly
    from the environment.

In this way the following sequence of operations becomes predictable:

```
    $ juju deploy --constraints mem=2G wordpress
    $ juju set-constraints --service wordpress mem=3G
    $ juju add-unit wordpress -n 2
```

...in that exactly one machine will be provisioned with the first set of
constraints, and exactly two of them will be provisioned using the second
set. This is much friendlier to the users than delaying the unit constraint
capture and potentially suffering subtle and annoying races.

Subordinate units cannot have constraints, because their deployment is
controlled by their principal units. There's only ever one machine to which
that subordinate could (and must) be deployed, and to restrict that further
by means of constraints will only confuse people.

Placement
---------

Placement is the term given to allocating a unit to a specific machine.
This is achieved with the `--to` option in the `deploy` and `add-unit`
commands.

In addition, it is possible to specify directives to `add-machine` to
allocate machines to specific instances:

  - in a new container, possibly on an existing machine (e.g. `add-machine lxc:1`)
  - by using an existing host (i.e. `add-machine ssh:user@host`)
  - using provider-specific features (e.g. `add-machine zone=us-east-1a`)

At the time of writing, the currently implemented provider-specific placement directives are:

  - Availability Zone: both the AWS and OpenStack providers support `zone=<zone>`, directing the provisioner to start an instance in the specified availability zone.
  - MAAS: `<hostname>` directs the MAAS provider to acquire the node with the specified hostname.

Availability Zone Spread
------------------------

For Juju providers that know about Availability Zones, instances will be automatically spread across the healthy availability zones to maximise service availability. This is achieved by having Juju:

  - be able to enumerate each of the availability zones and their current status,
  - calculate the "distribution group" for each instance at provisioning time.

The distribution group of a nascent instance is the set of instances for which the availability zone spread will be computed. The new instance will be allocated to the zone with the fewest members of its group.

Distribution groups are intentionally opaque to the providers. There are currently two types of groups: state servers and everything else. State servers are always allocated to the same distribution group; other instances are grouped according to the units assigned at provisioning time. A non-state server instance's group consists of all instances with units of the same services.

At the time of writing, there are currently three providers providers supporting automatic availability zone spread: Microsoft Azure, AWS, and OpenStack. Azure's implementation is significantly different to the others as it contains various restrictions relating to the imposed conflation of high availability and load balancing.

The AWS and OpenStack implementations are both based on the `provider/common.ZonedEnviron` interface; additional implementations should make use this if possible. There are two components:

  - unless a placement directive is specified, the provider's `StartInstance` must allocate an instance to one of the healthy availability zones. Some providers may restrict availability zones in ways that cannot be detected ahead of time, so it may be necessary to attempt each zone in turn (in order of least-to-most populous);
  - the provider must implement `state.InstanceDistributor` so that units are assigned to machines based on their availability zone allocations.

Machine Status and Provisioning Errors (current)
------------------------------------------------

In the light of time pressure, a unit assigned to a machine that has not been
provisioned can be removed directly by calling `juju destroy-unit`. Any
provisioning error can thus be "resolved" in an unsophisticated but moderately
effective way:

```
    $ juju destroy-unit borken/0
```

...in that at least broken units don't clutter up the service and prevent its
removal. However:

```
    $ juju destroy-machine 1
```

...does not yet cause an unprovisioned machine to be removed from state (whether
directly, or indirectly via the provisioner; the best place to implement this
functionality is not clear).

Machine Status and Provisioning Errors (WIP)
--------------------------------------------

[TODO: figure this out; not yet implemented, somewhat speculative... in
particular, use of "resolved" may be inappropriate. Consider adding a
"retry" CLI tool...]

When the provisioner fails to start a machine, it should ensure that (1) the
machine has no instance id set and (2) the machine has an error status set
that communicates the nature of the problem. This must be visible in the
output of `juju status`; and we must supply suitable tools to the user so
as to allow her to respond appropriately.

If the user believes a machine's provisioning error to be transient, she can
do a simple `juju resolved 14` which will set some state to make machine 14
eligible for the provisioner's attention again.

It may otherwise be that the unit ended up snapshotting a service/environ
config pair that really isn't satsifiable. In that case, the user can try
(say) `juju resolved 14 --constraints "mem=2G cpu-power=400"`, which allows
her to completely replace the machine's constraints as well as marking the
machine for reprovisioning attention.
