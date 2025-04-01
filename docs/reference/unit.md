(unit)=
# Unit

> See also: {ref}`manage-units`

In Juju, a **unit** is a deployed {ref}`charm <charm>`.

<!--
  running instance of an {ref}`application <application>`. 
-->

An application’s units occupy {ref}`machines <machine>`. 

Simple applications may be deployed with a single application unit, but it is possible for an individual application to have multiple units running on different machines. For example, one may deploy a single MongoDB application, and specify that it should run three units (with one machine per unit) so that the replica set is resilient to failures. 


<!--
This is represented in the diagram below:

![units](https://assets.ubuntu.com/v1/244e4890-juju-machine-units.png)
-->

A unit is always named on the pattern `<application>/<unit ID>`, where `<application>` is the name of the application and the `<unit ID>` is its ID number or, for the leader unit, the keyword `leader`. For example, `mysql/0` or `mysql/leader`. Note: the number designation is a static reference to a unique entity whereas the `leader` designation is a dynamic reference to whichever unit happens to be elected by Juju to be the leader . 


<!--CHECK AND ADD: An application unit is the smallest entity managed by Juju.-->

(leader-unit)=
## Leader unit

<!--TODO: Rewrite to improve perspicuity.-->

In Juju, a **leader** (or {ref}`application <application>` leader) is the application {ref}`unit <unit>` that is the authoritative source for an application's status and configuration. 

All units for a given application share the same charm code, the same relations, and the same user-provided configuration but the leader unit is different in that it is responsible for managing the lifecycle of the application as a whole. 

Every application is guaranteed to have at most one leader at any given time. {ref}`Unit agents <unit-agent>` will each seek to acquire leadership, and maintain it while they have it or wait for the current leader to drop out. 

Internally, even though the replica set shares the same user-provided configuration, each unit may be performing different roles within the replica set, as defined by the {ref}`charm <charm>`.

The leader is denoted by an asterisk in the output to `juju status`.
