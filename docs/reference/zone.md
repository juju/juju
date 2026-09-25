---
myst:
  html_meta:
    description: "Availability zone reference: constraint and placement directive for distributing resources across cloud zones for redundancy. The zone value and where it lives."
---

(zone)=
# Zone

A(n availability) **`zone`** is a  {ref}`constraint <constraint>` or a {ref}`placement directive <placement-directive>` that can be used to customise where the hardware spawned by Juju is provisioned in order to achieve better redundancy in case of an outage.

(the-zone-record)=
(the-zones-records)=
## The zone's records

### The zone's identity

A zone is a **value, not an entity**: it is one entry in the cloud's
list of availability zones. What persists is where it is *used* -- a
machine's cloud-instance record names the zone its instance landed
in (an empty pointer until the cloud assigns one).

(the-zone-in-the-data-model)=
### The zone in the data model

The zone lookup is a cloud-side table; the model stores only the
pointer on the machine's cloud instance (see
{ref}`the machine in the data model
<the-machine-in-the-data-model>`). The two ways to name a zone are
the `zones` {ref}`constraint <constraint-zones>` (a range) and the
`zone=` {ref}`placement directive <placement-directive-zone>` (one
zone).

When passed as a constraint you may specify a range of zones (via the {ref}`constraint-zones` key) whereas when passed as a placement directive you may only specify one zone (via the {ref}`placement-directive-zone` key). If you do both -- that is, there is overlap -- the placement directive takes precedence.

(the-zone-states)=
### Zone states

Not applicable -- a zone is a cloud fact, cached in the model: it is
read, never written or transitioned.

(the-zone-operations)=
## The zone's machinery

A zone has no machinery of its own: it is a cached cloud fact -- the
zone list comes from the provider, and what persists is where a
machine's instance landed.

### Zone operations

Not applicable -- zones are not operated on in Juju; they are
consumed at provisioning time by the constraints and directives that
name them (see {ref}`machine provisioning <machine-provisioning>`).

(the-zone-watchers)=
### Zone watchers

Not applicable -- the zone list comes from the cloud provider; it is
not model state with a change stream.

(the-zone-rules-and-errors)=
## Zone rules and errors

- valid zone values are the cloud's own zone names -- Juju validates
  nothing beyond what the provider accepts at provisioning time;

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>`
```

(related-entities-zone)=
## Entities related to the zone

- **Constraints** name zone ranges (see {ref}`constraint <constraint>`).
- **Placement directives** name one zone (see
  {ref}`placement directive <placement-directive>`).
- **Machines** record the zone they landed in, on their cloud
  instance (see {ref}`machine <machine>`).
