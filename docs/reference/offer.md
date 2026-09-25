---
myst:
  html_meta:
    description: "Juju offer reference: applications made available for cross-model relations. The offer record, the offer in the data model, offer operations, and rules."
---

(offer)=
# Offer
```{audience} user
```

```{ibnote}
See also: {ref}`manage-offers`
```

In Juju, an **offer** represents an {ref}`application <application>` that has been made available for {ref}`cross-model relations <cross-model-relation>`.

When you are integrating an application with an offer, what you're doing is consume + integrate, where consume = validate that your user has permission to consume the offer + create a local application proxy for the application and integrate is the usual local integrate.

(the-offers-records)=
## The offer's records

(the-offer-record)=
### The offer's identity

In the offering model's database, an offer is a record: its name (by
default the application's name) and its UUID, its
{ref}`endpoints <application-endpoint>` (an offer's endpoints all
belong to one application -- the schema enforces it), and one
**connection** record per consumer -- the consuming side's remote
relation and its consumer details. The offer's users (who may consume
it) live in the controller database as permission rows, not in the
model (see {ref}`user <user>`).

An offer is identified by its **offer URL**,
`[<source>:][<qualifier>/]<model-name>.<offer-name>`, optionally with
a `:<relation-endpoint>` suffix; the offer name follows the
application-name rules.

(the-offer-origins)=
```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR)
:alt: Two model databases side by side. In the offering model: relation and endpoint records belonging to the offer, offer and offer-connection records, external controller record. In the consuming model: application, relation and endpoint records for the proxy application, remote application record. Arrows follow the foreign keys from each side's records into the shared offer machinery.
:caption: Topology: The cross-model relation, record by record. The offering side stores the offer and its connections; the consuming side stores a proxy application and a remote-application record; both sides agree on the endpoint and relation records that carry the actual relation data. Nothing is shared between the two model databases except the offer URL and credentials.
```

(the-offer-in-the-data-model)=
### The offer in the data model

The offer's stored records are the three drawn above: the `offer` row
(name, UUID), the `offer_endpoint` rows (the offer's endpoints, each a
pointer into the application's endpoint records, with the
single-application trigger), and the `offer_connection` rows (one per
consumer, naming the remote relation and the consumer). A read view
joins them with the connection counts; the offer's access control is
controller-side: creating an offer grants its owner admin access and
everyone read access as permission rows on the offer's UUID.

(the-offer-states)=
### Offer states

An offer has no life column and no state machine: the record is static
from its creation until it is removed, and its only moving quantity
-- the number of active connections -- is derived from the
connections' relations, not stored.

(types-of-offer)=
### Types of offer

An offer has no subtypes: it is one record shape -- an application's
endpoints published for consumption -- and Juju adds no kind column.

(the-offers-machinery)=
## The offer's machinery

An offer has no machinery of its own: it is a static record the
controller serves -- creating, consuming and removing it are record
writes, and what moves around an offer runs in the cross-model
relation machinery.

(the-offer-operations)=
### Offer operations
#### Creating an offer

Creating an offer (for example, `juju offer mysql:mysql`) publishes an
application's endpoint: the offer is named after the application by
default, an identical offer (same application and endpoints) is
rejected rather than duplicated, and the owner-plus-everyone access
rows are written controller-side.

#### Consuming an offer

Consuming validates the user's permission and creates the local proxy
application and its remote-application record (the consume details
service hands the consuming side its half; see the CMR view above and
{ref}`cross-model relation <cross-model-relation>`).

#### Removing an offer

Removing an offer refuses while the offer still has connections or
active relations, unless the removal is forced; the removal also
deletes the offer's access rows.

There is no update operation: an existing offer's endpoints cannot be
changed -- deploy a new offer instead.

(the-offer-watchers)=
### Offer watchers
```{audience} juju-dev
```

The offer has no watch surfaces of its own: nothing polls or watches
the offer record. What moves around an offer -- the remote relation's
changes, the secrets shared across it -- has its own watchers in the
cross-model relation machinery (see
{ref}`cross-model relation <cross-model-relation>`).

(the-offer-rules-and-errors)=
## Offer rules and errors
```{audience} charm-dev
```

The rules an **offer URL** must satisfy:

- the form is `[<source>:][<qualifier>/]<model-name>.<offer-name>`
  with an optional `:<relation-endpoint>` suffix;
- the model name must be a valid model name, and the offer name the
  application-name rules (lowercase letters, digits and hyphens, no
  leading hyphen).

The rules an **offer mutation** must satisfy:

- an offer's endpoints must name endpoints of one application (the
  schema trigger enforces it);
- an identical offer -- same application and endpoints -- is not
  created twice (`offer already exists`);
- a consumed offer is not consumed again by the same model
  (`offer already consumed`);
- removal requires force while connections or relations remain
  (`offer has relations`).

The errors that encode them: `offer not found`,
`offer already exists`, `offer already consumed`, `offer URL not
valid`, `missing endpoints`, `offer has relations`.

(related-entities-offer)=
## Entities related to the offer

- **Applications** are what an offer publishes -- one application's
  endpoints (see {ref}`application <application>`).
- **Cross-model relations** are what an offer enables; the consuming
  side runs a proxy application against a remote-application record
  (see {ref}`cross-model relation <cross-model-relation>`).
- **Users** hold the offer's access levels, stored as controller-side
  permission rows (see {ref}`user <user>`).
- **External controllers** carry the far model's identity, which the
  offer URL's source names (see
  {ref}`external controller <controller>`).
