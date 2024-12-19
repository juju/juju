(provisioning)=
# Provisioning

Provisioning in Juju is performed by one of two separate workers:
- The [compute provisioner](https://github.com/juju/juju/blob/main/internal/worker/computeprovisioner/doc.go),
which is responsible for provisioning machine instances on the underlying
provider.
- The [container provisioner](https://github.com/juju/juju/blob/main/internal/worker/containerprovisioner/doc.go),
which is responsible for creating containers managed by the container
broker on the underlying machine.

Juju splits the resource creation in two stages. When a new machine
is being created (either by the user explicitly or because a new unit
is deployed) the machine is first stored in the database, and then
one of the provisioners (compute or container) workers will pick it
up and start the provisioning process. Both workers will gather all the
necessary information and then create a [provisioner_task](https://github.com/juju/juju/blob/main/internal/provisionertask/provisioner_task.go)
worker. Provisioner task workers contain all the business logic for creating and updating
instances, as well as keeping track of all the instances.

The following sections will describe in more detail the different
pieces that participate in the provisioning process.

## Bases

Individual charms are released for different possible target bases; Juju
should guarantee that charms for base X are only ever run on base X.
Every application, unit, and machine has a base that's set at creation time and
subsequently immutable. Units take their base from their service, and can
only be assigned to machines with matching bases.

Subordinate units cannot be assigned directly to machines; they are created
by their principals, on the same machine, in response to the creation of
subordinate relations. We therefore restrict subordinate relations such that
they can only be created between services with matching bases.

## Constraints

Constraints are stored for models, services, units, and machines, but
unit constraints are not currently exposed because they're not needed outside
state, and are likely to just cause trouble and confusion if we expose them.

From the point of view of a user, there are model constraints and service
constraints, and sensible manipulations of them lead to predictable unit
deployment decisions. The mechanism is as follows:

  * when a unit is added, the current model and service constraints
    are collapsed into a single value and stored for the unit. (To be clear:
    at the moment the unit is created, the current service and model
    constraints will be combined such that every constraint not set on the
    service is taken from the model, or left unset if not specified
    at all).
  * when a machine is being added in order to host a given unit, it copies
    its constraints directly from the unit.
  * when a machine is being added without a unit associated -- for example,
    when adding additional controllers -- it copies its constraints directly
    from the model.

In this way the following sequence of operations becomes predictable:

```
    $ juju deploy --constraints mem=2G wordpress
    $ juju set-constraints --application wordpress mem=3G
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

## Placement directives

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

## Availability zones

For Juju providers that know about [Availability zones](https://juju.is/docs/juju/availability-zone),
instances will be automatically spread across the healthy availability zones
to maximise service availability. This is achieved by having Juju:

  - be able to enumerate each of the availability zones and their current status,
  - calculate the "distribution group" for each instance at provisioning time.

The distribution group of a nascent instance is the set of instances for which
the availability zone spread will be computed. The new instance will be
allocated to the zone with the fewest members of its group.

Distribution groups are intentionally opaque to the providers. There are
currently two types of groups: controllers and everything else. controllers are
always allocated to the same distribution group; other instances are grouped
according to the units assigned at provisioning time. A non-controller
instance's group consists of all instances with units of the same services.

Unless a placement directive is specified, the provider's `StartInstance` must
allocate an instance to one of the healthy availability zones. Some providers
may restrict availability zones in ways that cannot be detected ahead of time,
so it may be necessary to attempt each zone in turn (in order of least-to-most
populous);
