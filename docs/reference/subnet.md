---
myst:
  html_meta:
    description: "Juju subnet reference: IP address ranges in CIDR notation, grouped into network spaces for application networking. The subnet record, its space membership, and its rules."
---

(subnet)=
# Subnet
```{audience} user
```

```{ibnote}
See also: {ref}`manage-subnets`
```

A **subnet** is a range of IP addresses in CIDR notation.

(the-subnets-records)=
(the-subnet-record)=
(the-subnet-in-the-data-model)=
(the-subnet-states)=
## The subnet in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Subnet attributes
:alt: The subnet's stored tables as an entity-relationship slice: the subnet record at the centre with its uuid, cidr, vlan tag and space pointer; the space record it joins above; the provider identity satellites east; the availability-zone membership record south. Every arrow starts at the foreign-key column that stores the pointer.
:caption: Entity relationship diagram: The subnet's stored records and every foreign key between them -- each arrow starts at the fk column that stores the pointer (the only directionality the storage layer has). The space, availability_zone and provider_network records are drawn as name-only chips: their stories are their own pages'.
```

In the model database a subnet is a **cached cloud fact**: the
provider discovers the CIDR range and Juju stores what it learned --
the range itself (`cidr`), the VLAN tag when the cloud reports one
(`vlan_tag?`, nullable because not every subnet is tagged). The
`provider_subnet` satellite carries the provider's own identifier for
the subnet, and the `provider_network_subnet` join names the provider
network the subnet sits in: both keep the cloud's vocabulary for the
same fact, so Juju can talk to the provider about a subnet it did not
create. The space the subnet joins is Juju's own grouping; the zone
membership (`availability_zone_subnet`, its composite primary key is
the membership itself) is the cloud's placement fact, adopted as
found (see {ref}`space <space>`).

The identity pair: the primary key (`subnet.uuid`) is the join
handle -- it exists so the space pointer, the provider satellites and
the zone membership have something to point at. The subnet has no
unique natural key of its own: the client addresses a subnet by its
CIDR (as listed by `juju subnets`), and the cloud's handle for it is
the 1:1 `provider_subnet.provider_id`. The unique names in the
picture belong to the neighbours: `space.name` and
`availability_zone.name`.

Every foreign key is an assertion the record holds:
`subnet.space_uuid` says this subnet belongs to at most one space --
nullable, an honest absence the schema states (a subnet need not be
grouped yet); moving it between spaces rewrites the single pointer
(see {ref}`space operations <the-space-operations>`). Nothing
transitions: the record has no life column and no status table -- a
subnet is discovered, listed, and moved; there is no state to read
off the diagram and no type vocabulary behind it.

(the-subnets-machinery)=
## The subnet's machinery

A subnet has no machinery of its own: it is a cached cloud fact the
network machinery reads -- the one watch surface (subnet changes)
reports the mirroring, not any machinery of the subnet's.

(the-subnet-operations)=
### Subnet operations

Not applicable in the create/update sense: subnets are the cloud's.
Juju lists them, adopts them on the space reload, and moves them
between spaces (see {ref}`space <space>`).

(the-subnet-watchers)=
### Subnet watchers
```{audience} juju-dev
```

The network domain's one watch surface is **subnet changes** -- the
provisioning and address machinery's input (see
{ref}`space watchers <the-space-watchers>`).

Every watcher fires once immediately when it is created -- the initial
query is the baseline snapshot -- and again on each qualifying change
(see {ref}`the watcher pattern <watchers>`).

(the-subnet-rules-and-errors)=
## Subnet rules and errors
```{audience} charm-dev
```

- the CIDR must parse as a CIDR range, and the VLAN tag (when
  present) must be a valid VLAN id;
- a subnet belongs to at most one space -- moving it between spaces
  rewrites the single pointer.

(related-entities-subnet)=
## Entities related to the subnet

- **Spaces** group subnets -- the membership pointer is the subnet's
  (see {ref}`space <space>`).
- **Bindings** name spaces, which select subnets (see
  {ref}`application endpoint <application-endpoint>`).
- **Machines** get their addresses from the subnets the bindings
  select (see {ref}`machine <machine>`).
