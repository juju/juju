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
## The subnet in the declaration layer

No client creates or destroys a subnet: a subnet enters the model by
adoption, and the client's writes on one are space-pointer moves:

- **Adoption:** The provider reload discovers the cloud's subnets
  (and its spaces, where the cloud exposes its own grouping); the
  model import brings a migrating model's subnets across. A subnet
  adopted with no space of its own lands in the default `alpha`
  space.
- **Moving:** Naming a space around CIDRs adopts the subnets that
  carry them into the new space, and a move rewrites the subnet's
  space pointer (see {ref}`space operations
  <the-space-operations>`). The move's constraint guards are the
  rules below.

(the-subnet-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - A subnet's range is a CIDR in canonical form: the parsed network
    spells the input back, so `10.0.0.1/24` fails where `10.0.0.0/24`
    stands. The ranges the reload will not carry are the
    interface-local multicast, link-local multicast and link-local
    unicast blocks.
  - A move is guarded: without forcing it is refused when it would
    leave a machine without addresses in a space its constraints or
    an {ref}`application endpoint <application-endpoint>` binding
    requires, or when it would give a machine addresses in a space a
    constraint excludes; forced, the violations are logged as
    warnings and the move proceeds.
  - The VLAN tag is stored as the provider reports it: the write path
    runs no tag check and the schema states no range for it. The
    0-4094 rule is the core subnet type's, not a model gate.
- **Errors:**
  - **`subnet CIDR is not valid`:** Triggered when an operation that
    names CIDRs takes a range that does not parse as a canonical
    CIDR; the space-creation gate parses every range it is given.
    Remediation: write the range in canonical CIDR form.
  - **`space not found`:** Triggered when a move names a target space
    the model does not have; the move resolves its target by name
    before it writes. Remediation: check the space name against the
    model's spaces.

(the-subnet-in-the-data-model)=
## The subnet in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Subnet attributes
:alt: The subnet's stored records as an entity-relationship slice: the subnet record at the centre with its uuid, cidr, vlan tag and space pointer; the space record it joins above; the provider identity satellites east; the availability-zone membership record south. Each line is a stored pointer; 1/m at each end; the space pointer's line is dashed (the fk is nullable).
:caption: Entity relationship diagram: The subnet's stored records and the schema associations between them -- each line starts at the fk column that holds the pointer (the only directionality the storage layer has; the DDL and the fk: badges own it -- the drawing states the association, 1/m at each end, dashed = the record may be absent). The space, availability_zone and provider_network records are drawn as name-only chips: their stories are their own pages'.
```

In the model database a subnet is a **cached cloud fact**: the
provider discovers the CIDR range and Juju stores what it learned:
the range itself (`cidr`), the VLAN tag when the cloud reports one
(`vlan_tag`, nullable because not every subnet is tagged). The
`provider_subnet` satellite carries the provider's own identifier
for the subnet, and the `provider_network_subnet` join names the
provider network the subnet sits in: both keep the cloud's
vocabulary for the same fact, so Juju can talk to the provider about
a subnet it did not create. The space the subnet joins is Juju's own
grouping; the zone membership (`availability_zone_subnet`, its
composite primary key is the membership itself) is the cloud's
placement fact, adopted as found (see {ref}`space <space>`).

The record set over the DDL (`0009-subnet.sql`, `0008-space.sql`):

- **`subnet`:** The range and its grouping: a `uuid` primary key
  (the join handle), the `cidr` range, the nullable `vlan_tag`, and
  the nullable `space_uuid` pointer to {ref}`space <space>`. There
  is no unique natural key: the CIDR is not unique (distinct
  provider networks can present the same range twice), so the client
  addresses a subnet by its CIDR and the cloud by its provider id.
- **`provider_subnet`:** The cloud's own identifier: `provider_id`
  primary key, non-empty by a CHECK, and one per subnet (the unique
  index on `subnet_uuid`).
- **`provider_network` and `provider_network_subnet`:** The cloud's
  network grouping: the join's primary key is the subnet's `uuid`,
  so a subnet sits in at most one provider network.
- **`availability_zone` and `availability_zone_subnet`:** The
  cloud's placement: a {ref}`zone <zone>`'s `name` is unique, and
  the membership is a composite primary key, the zone on one side
  and the subnet on the other: a subnet sits in many zones, a zone
  holds many subnets.
- **`v_space_subnet`:** The flattened view the reads use: every
  subnet joined with its space name, its provider satellites and
  its zone names, so one record answers the listing.
- The consumer outside the slice: **`ip_address`** carries the
  subnet pointer (`subnet_uuid`, nullable: the Kubernetes
  provider's pod IPs can lack a discovered subnet, the schema's own
  comment); the {ref}`machine <machine>`'s addresses and the
  container lookups read through it.

The identity pair: the primary key (`subnet.uuid`) is the join
handle, so the space pointer, the provider satellites and the zone
membership have something to point at. The unique names in the
picture belong to the neighbours: `space.name` and
`availability_zone.name`.

Every foreign key is an assertion the record holds: the subnet record
holds the space pointer (`subnet.space_uuid`); the provider_subnet
record holds the subnet pointer; the join records hold one pointer
each
(`availability_zone_subnet` both of its neighbours'). The space
assertion says this subnet belongs to at most one space: nullable,
an honest absence the schema states (a subnet need not be grouped
yet); moving it between spaces rewrites the single pointer (see
{ref}`space operations <the-space-operations>`). Nothing
transitions: the record has no life and no status vocabulary; a
subnet is adopted, listed, and moved; there is no state to read off
the diagram and no type vocabulary behind it.

(the-subnet-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - A subnet belongs to at most one space or to none
    (`subnet.space_uuid` is nullable); a move rewrites the pointer,
    and deleting the space it points at rewrites the pointer to
    `alpha` (the {ref}`space <space>` side carries the rest of the
    removal).
  - Adoption is idempotent: adding a subnet whose provider id the
    model already knows is tolerated, not an error; the reload
    upserts, updating a known subnet's space pointer and adding an
    unknown subnet with its satellites in the same transaction.
  - No path deletes a subnet in the current code: the removal
    surface exists (the state deletes the record and its satellites in
    one transaction) but has no non-test caller, and the reload's
    own comment says it does not delete subnets the provider
    dropped.
- **Errors:**
  - **`subnet not found`:** Triggered when a subnet is queried by
    its UUID or by the cloud's provider id and no record answers, when
    an add-space operation names a CIDR no subnet carries, and when
    a device's address lookup has no subnet to name. Remediation:
    check the identifier against the listing.

(the-subnets-machinery)=
## The subnet in the execution layer

A subnet has no machinery of its own: nothing executes a subnet.
What runs is the adoption, split by owner:

- **The provider reload:** The controller asks the provider for its
  networking shape. Where the cloud exposes its own space notion, the
  discovered spaces are saved (new ones added, their subnets
  upserted into them) and the spaces the cloud no longer reports are
  removed, their subnets falling to `alpha`; a space the removal's
  checks refuse is left in place with a warning. Where the cloud has
  no space notion, the discovered subnets are upserted into the
  default `alpha` space, the multicast and link-local ranges dropped
  on the way, and a range that does not parse fails the reload.
- **The model import:** A migrating model's spaces and subnets are
  imported explicitly: each subnet is linked to its space by
  identifier, not by CIDR (the CIDR association is ambiguous when
  distinct provider networks present the same range); a Kubernetes
  model, which has no spaces or subnets of its own, gets the
  fallback subnets seeded for referential integrity.
- **The unused surfaces:** The service exposes a subnet update (a
  space-pointer rewrite) and a subnet removal; neither has a
  non-test caller in the current code. The surfaces are stated, not
  narrated: nothing invokes them.

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
