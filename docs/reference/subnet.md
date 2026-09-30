---
myst:
  html_meta:
    description: "Juju subnet reference: IP address ranges in CIDR notation, adopted from the cloud and grouped into spaces. The subnet in the declaration layer (CIDR ranges, adoption, moves between spaces), the subnet record and its provider satellites in the persistence layer, and the execution layer's adoption machinery and subnet changes watch surface."
---

(subnet)=
# Subnet

```{ibnote}
See also: {ref}`manage-subnets`
```

In Juju, a **subnet** is a range of IP addresses in CIDR notation. A
subnet is the cloud's fact: the provider discovers the range, and the
model adopts what it learned, keeping the cloud's own identifiers
beside its record. Subnets are grouped into {ref}`spaces <space>`, the
grouping the client shapes; a subnet's own story is adoption and
membership.

(the-subnet-operations)=
(the-subnet-declaration-rules)=
## The subnet in the declaration layer

No client creates or destroys a subnet: a subnet enters the model by
adoption, and the client's writes on one are space-pointer moves:

- **Adoption:** The provider reload discovers the cloud's subnets (and its spaces, where the cloud exposes its own grouping); the model import brings a migrating model's subnets across. A subnet adopted with no space of its own lands in the default `alpha` space.
  - *Rule:* A subnet's range is a CIDR in canonical form: the parsed network spells the input back, so `10.0.0.1/24` fails where `10.0.0.0/24` stands. The ranges the reload will not carry are the interface-local multicast, link-local multicast and link-local unicast blocks.
- **Moving:** Naming a space around CIDRs adopts the subnets that carry them into the new space, and a move rewrites the subnet's space pointer (see {ref}`space operations <the-space-operations>`).
  - *Rule:* A move is guarded. Without forcing, it is refused when it would leave a machine without addresses in a space its constraints or an {ref}`application endpoint <application-endpoint>` binding requires, or when it would give a machine addresses in a space a constraint excludes; forced, the violations are logged as warnings and the move proceeds.
  - *Related errors:*
    - **`subnet CIDR is not valid`**: *Trigger:* An operation that names CIDRs takes a range that does not parse as a canonical CIDR; the space-creation gate parses every range it is given. *Remediation:* Write the range in canonical CIDR form.
    - **`space not found`**: *Trigger:* A move names a target space the model does not have; the move resolves its target by name before it writes. *Remediation:* Check the space name against the model's spaces.

(the-subnet-in-the-data-model)=
(the-subnet-persistence-rules)=
## The subnet in the persistence layer

In the {ref}`model database <database>` a subnet is a **cached cloud fact**: the
provider discovers the CIDR range and Juju stores what it learned:
the range itself, with the VLAN tag when the cloud reports one (not
every subnet is tagged). A provider-identity record carries the
cloud's own identifier for the subnet, and the provider-network
membership names the provider network the subnet sits in: both keep
the cloud's vocabulary for the same fact, so Juju can talk to the
provider about a subnet it did not create. The space the subnet joins
is Juju's own grouping; the zone membership is the cloud's placement
fact, adopted as found (see {ref}`space <space>`).

The record set, in prose:

- **Primary entry:** A single record containing the subnet's essential attributes: the range, the VLAN tag and the space grouping. The primary key is the join handle, so the space pointer, the provider satellites and the zone membership have something to point at.
  - *Rule:* There is no unique natural key: the CIDR is not unique (distinct provider networks can present the same range twice), so the client addresses a subnet by its CIDR and the cloud by its provider id.
  - *Rule:* The VLAN tag is stored as the provider reports it: the write path runs no tag check and the schema states no range for it. The 0-4094 rule is the core subnet type's, not a model gate.
  - *Related error:*
    - **`subnet not found`**: *Trigger:* A subnet is queried by its UUID or by the cloud's provider id and no record answers; an add-space operation names a CIDR no subnet carries; or a device's address lookup has no subnet to name. *Remediation:* Check the identifier against the listing.
- **Space pointer:** The subnet record holds the space pointer, an assertion that this subnet belongs to at most one space. It is nullable, an honest absence the schema states (a subnet need not be grouped yet); moving the subnet between spaces rewrites the single pointer (see {ref}`space operations <the-space-operations>`).
  - *Rule:* Deleting the space the pointer names rewrites the pointer to `alpha` (the {ref}`space <space>` side carries the rest of the removal).
- **Provider identity:** A satellite record, one per subnet, keyed by the cloud's own identifier, which the schema keeps non-empty; it is how the cloud and Juju refer to the same fact.
- **Provider network membership:** One record per subnet. The membership is the whole record, so a subnet sits in at most one provider network.
- **Zone membership:** A subnet sits in zones by membership records. A {ref}`zone <zone>`'s name is unique, and each membership pairs one zone with one subnet: a subnet sits in many zones, a zone holds many subnets.
- **Derived view:** The reads go through a view: every subnet joined with its space name, its provider satellites and its zone names, so one record answers the listing.
- **Consumer outside the slice:** The {ref}`machine <machine>`'s address records carry the subnet pointer (nullable: the Kubernetes provider's pod IPs can lack a discovered subnet, the schema's own comment); the container lookups read through them.
- **States:** Nothing transitions: the record has no life and no status vocabulary; a subnet is adopted, listed, and moved; there is no state to read off the record and no type vocabulary behind it. The unique names belong to the neighbours: the space's name and the zone's name.

(the-subnets-machinery)=
## The subnet in the execution layer

A subnet has no machinery of its own: nothing executes a subnet.
What runs is the adoption, split by owner:

- **The provider reload:** The controller asks the provider for its networking shape. Where the cloud exposes its own space notion, the discovered spaces are saved (new ones added, their subnets upserted into them) and the spaces the cloud no longer reports are removed, their subnets falling to `alpha`; a space the removal's checks refuse is left in place with a warning. Where the cloud has no space notion, the discovered subnets are upserted into the default `alpha` space, the multicast and link-local ranges dropped on the way, and a range that does not parse fails the reload.
  - *Rule:* Adoption is idempotent: adding a subnet whose provider id the model already knows is tolerated, not an error; the reload upserts, updating a known subnet's space pointer and adding an unknown subnet with its satellites in the same transaction.
- **The model import:** A migrating model's spaces and subnets are imported explicitly: each subnet is linked to its space by identifier, not by CIDR (the CIDR association is ambiguous when distinct provider networks present the same range); a Kubernetes model, which has no spaces or subnets of its own, gets the fallback subnets seeded for referential integrity.
- **The unused surfaces:** The service exposes a subnet update (a space-pointer rewrite) and a subnet removal; neither has a non-test caller in the current code. The surfaces are stated, not narrated: nothing invokes them.
  - *Rule:* No path deletes a subnet in the current code: the removal surface exists (the state deletes the record and its satellites in one transaction) but has no non-test caller, and the reload's own comment says it does not delete subnets the provider dropped.

(the-subnet-watchers)=
### Subnet watchers

The network domain exposes one watch surface over subnets: **subnet
changes**. The watcher's namespace is the subnet record's change
stream, its initial query lists every subnet UUID, and its filter
narrows the stream to the watcher's own subnet set (an empty set
passes everything, the legacy compatibility form). The consumer in
the current code is the firewaller: the firewaller facade subscribes,
and the firewaller worker reacts to the subnets' changes. The space
side has no watcher of its own: a space's change is implied by its
subnets' moves (see {ref}`space watchers <the-space-watchers>`).

Every watcher fires once immediately when it is created, the initial
query being the baseline snapshot, and again on each qualifying
change (see {ref}`the watcher pattern <watchers>`).
