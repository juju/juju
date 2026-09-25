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
## The subnet's records

(the-subnet-record)=
### The subnet's identity

In the model database, a subnet is a record cached from the cloud:
its CIDR, its VLAN tag, and the {ref}`space <space>` it belongs to.
Subnets can be grouped to form a {ref}`space <space>`. A subnet can only be in one space.

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:no-legend:
:caption: Topology: A subnet is a CIDR range that belongs to 0..1 space; the application default binding and per-endpoint bindings point at spaces.
:alt: Application record to space record to subnet record.
```

(the-subnet-in-the-data-model)=
### The subnet in the data model

The subnet row carries the space pointer (a subnet belongs to at most
one space), the provider's own identifier for it, and the VLAN tag.
Subnets are discovered from the provider -- the space reload adopts
the cloud's view -- and moved between spaces by the
{ref}`space operations <the-space-operations>`.

(the-subnet-states)=
### Subnet states

Not applicable -- a subnet is a cached cloud fact: discovered,
listed, moved between spaces; nothing transitions.

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

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

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
