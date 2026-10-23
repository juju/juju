---
myst:
  html_meta:
    description: "Availability zone reference: constraint and placement directive for distributing resources across cloud zones for redundancy. The zone value and where it lives."
---

(zone)=
# Zone

A(n availability) **`zone`** is a  {ref}`constraint <constraint>` or a {ref}`placement directive <placement-directive>` that can be used to customise where the hardware spawned by Juju is provisioned in order to achieve better redundancy in case of an outage.

(the-zone-declaration)=
(the-zone-rules-and-errors)=
## The zone in the declaration layer

A zone is discovered, not declared: no client adds or removes one. You name a zone in the requests that place compute.

- **Constraint:** When passed as a constraint you may specify a range of zones (via the {ref}`constraint-zones` key).
- **Placement directive:** When passed as a placement directive you may only specify one zone (via the {ref}`placement-directive-zone` key).
- **Overlap:** If you do both (that is, there is overlap), the placement directive takes precedence.
  - *Rule:* Valid zone values are the cloud's own zone names; Juju validates nothing beyond what the provider accepts at provisioning time.

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>`
```

(the-zone-record)=
(the-zones-records)=
(the-zone-in-the-data-model)=
(the-zone-states)=
(related-entities-zone)=
## The zone in the persistence layer

A zone is a **cached cloud fact**: the model database keeps the cloud's availability-zone names and where they are used.

- **Zone record:** One entry per availability zone, with a model-unique name.
- **Subnet memberships:** Join records pairing a zone with a subnet, so a subnet sits in many zones and a zone holds many subnets (see {ref}`the subnet's records <the-subnet-in-the-data-model>`).
- **Machine pointer:** A machine's cloud-instance record names the zone its instance landed in; the pointer stays empty until the cloud assigns one (see {ref}`the machine's records <the-machine-in-the-data-model>`).
  - *Related error:*
    - **`availability zone not found`**: *Trigger:* Reading the zone of a machine whose cloud instance has no zone set. *Remediation:* Wait until the cloud has reported the instance's zone.
- **Neighbours:** {ref}`Constraints <constraint>` name zone ranges, {ref}`placement directives <placement-directive>` name one zone, and machines record the zone they landed in.
- **States:** A zone has no state machine: it is a cloud fact, cached in the model; it is read, never transitioned.

**Writers:** The zone entries arrive with the subnets: the network state inserts a zone and its subnet membership when it adopts a subnet from the provider (domain/network/state/subnet.go).

## The zone in the execution layer

A zone has no machinery of its own: it is a cached cloud fact; the zone list comes from the provider, and what persists is where a machine's instance landed.

(the-zone-operations)=
### Zone operations

Not applicable: zones are not operated on in Juju; they are consumed at provisioning time by the constraints and directives that name them (see {ref}`machine provisioning <the-machines-machinery>`).

(the-zone-watchers)=
### Zone watchers

Not applicable: the zone list comes from the cloud provider; it is not model state with a change stream of its own.
