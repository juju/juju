---
myst:
  html_meta:
    description: "Juju offer reference: applications made available for cross-model relations. The offer record, offer operations, and offer rules."
---

(offer)=
# Offer

```{ibnote}
See also: {ref}`manage-offers`
```

In Juju, an **offer** is an {ref}`application <application>`'s endpoints published for other models to consume as a {ref}`cross-model relation <cross-model-relation>`.

## The offer in the declaration layer

How clients publish an application's endpoints, consume other models' offers, and manage the access around them.

- **Creation:** Creating an offer publishes the named endpoints of an application under an offer URL; the offer is named after the application by default, and creating one requires {ref}`model admin access <user-access-model-admin>`.
- **Idempotence:** Creating an offer that already exists with the same application and the same endpoints succeeds without change; a different offer under an existing name is rejected, because offers are not updated.
- **Consumption:** Consuming an offer validates the consuming user's {ref}`consume access <user-access-offer-consume>`, then creates a proxy application in the consuming model and the records that connect the two models; the usual local application integration follows.
- **Removal:** Removing an offer stops while the offer still has connections, unless the removal is forced.
- **Access:** Creating an offer grants its owner admin access and everyone read access, both as permission rows in the controller database (see {ref}`the user access levels <user-access-levels>`).

```{ibnote}
See also: {ref}`Terraform Provider for Juju | Manage offers <tfjuju:manage-offers>`
```

(the-offer-declaration-rules)=
### Declaration rules and errors

- **Rules:**
  - An offer URL has the form `[<source>:][<qualifier>/]<model-name>.<offer-name>`, optionally with a `:<relation-endpoint>` suffix; the model name must be a valid model name, and the offer name follows the application-name rules.
  - An offer's endpoints cannot be changed after creation; deploy a new offer instead.
- **Errors:**
  - **`offer URL not valid`:** Triggered when an offer URL has no offer name. Remediation: use the full offer URL form.
  - **`offer already exists`:** Triggered when creating an offer whose name is already taken by a different offer, since offers cannot be updated. Remediation: remove the existing offer or use another name.
  - **`missing endpoints`:** Triggered when the endpoints named for the offer do not all exist on the application. Remediation: check the application's endpoint names.
  - **`offer already consumed`:** Triggered when a model consumes an offer it already consumes, the proxy application for that offer being registered already. Remediation: none; the model's connection to the offer exists.
  - **`offer has relations`:** Triggered when removing an offer that still has connections without forcing the removal. Remediation: remove the connected relations first, or force the removal.

## The offer in the persistence layer

```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR)
:alt: Two model databases side by side. In the offering model: relation and endpoint records belonging to the offer, offer and offer-connection records, external controller record. In the consuming model: application, relation and endpoint records for the proxy application, remote application record. Arrows follow the foreign keys from each side's records into the shared offer machinery.
:caption: Topology: The cross-model relation, record by record. The offering side stores the offer and its connections; the consuming side stores a proxy application and a remote-application record; both sides agree on the endpoint and relation records that carry the actual relation data. Nothing is shared between the two model databases except the offer URL and credentials.
```

In the offering model's database, an offer is a small record set (`0032-offer.sql`, `0034-cross-model-relation.sql`):

- **`offer`**: The identity pair: a `uuid` primary key and the offer's `name`.
- **`offer_endpoint`**: The join table pointing the offer at the {ref}`application endpoints <application-endpoint>` it publishes; a schema trigger keeps every endpoint of one offer on the same application.
- **`offer_connection`**: One row per consumer: the pointer to the consumer's remote relation and the consuming model's offer user.
- **`v_offer_detail`**: The read view joining the offer with its endpoints, the application, the charm, and the connection counts; total and active connections are derived here, not stored.
- **The consuming side:** In the consuming model's database, the proxy application is a native {ref}`application <application>` row, and an `application_remote_offerer` record ties it to the offer (the offer's UUID and URL, the offering controller), with its own status satellite.

The offer's users (who may consume it) live in the controller database as permission rows, not in the model (see {ref}`user <user>`). The offer's URL identifies it across controllers; its name is unique among offers.

An offer has no life column, no state machine, and no subtypes: the record is static from creation until removal, and its only moving quantity, the number of active connections, is derived from the connections' relations.

(the-offer-persistence-rules)=
### Persistence rules and errors

- **Rules:**
  - All of an offer's endpoints belong to one application; the schema trigger on `offer_endpoint` enforces it.
  - The connection counts are derived: `v_offer_detail` counts an offer's connections and its active connections from the `offer_connection` rows.
- **Errors:**
  - **`offer not found`:** Triggered when querying an offer by URL or UUID that does not exist. Remediation: verify the offer URL.

Writers: the cross-model relation service performs the writes; offering inserts the offer row with its endpoint rows and the controller-side access rows, consuming adds the connection record and the consuming side's records, and removing deletes the offer with its endpoint, connection, and access rows.

## The offer in the execution layer

An offer has no machinery of its own: it is a static record the controller serves; creating, consuming, and removing it are record writes, and what moves across an offer runs in the cross-model relation machinery (see {ref}`cross-model relation <cross-model-relation>`).

(the-offer-watchers)=
### Offer watchers

The offer has no watch surfaces of its own: nothing polls or watches the offer record. What moves across an offer, the remote relation's changes, has its own watchers in the cross-model relation machinery.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
