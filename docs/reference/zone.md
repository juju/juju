---
myst:
  html_meta:
    description: "Availability zone reference: constraint and placement directive for distributing resources across cloud zones for redundancy."
---

(zone)=
# Zone

A(n availability) **`zone`** is a  {ref}`constraint <constraint>` or a {ref}`placement directive <placement-directive>` that can be used to customise where the hardware spawned by Juju is provisioned in order to prevent better redundancy in case of an outage.

The value of the key consists of the zone(s) available for a given cloud.

When passed as a constraint you may specify a range of zones (via the {ref}`constraint-zones` key) whereas when passed as a placement directive you may only specify one zone (via the {ref}`placement-directive-zone` key). If you do both -- that is, there is overlap -- the placement directive takes precedence.

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>`
```

