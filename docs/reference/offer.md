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

(the-offer-declaration-rules)=
## The offer in the declaration layer

How clients publish an application's endpoints, consume other models' offers, and manage the access around them.

- **Offer URL:** An offer is addressed by its URL.
  - *Rule:* An offer URL has the form `[<source>:][<qualifier>/]<model-name>.<offer-name>`, optionally with a `:<relation-endpoint>` suffix. The model name must be a valid model name, and the offer name follows the application-name rules.
  - *Related error:*
    - **`offer URL not valid`**: *Trigger:* An offer URL that has no offer name. *Remediation:* Use the full offer URL form.
- **Creation:** Creating an offer publishes the named endpoints of an application under an offer URL; the offer is named after the application by default, and creating one requires {ref}`model admin access <user-access-model-admin>`.
  - *Rule:* An offer's endpoints cannot be changed after creation; deploy a new offer instead.
  - *Related errors:*
    - **`missing endpoints`**: *Trigger:* The endpoints named for the offer do not all exist on the application. *Remediation:* Check the application's endpoint names.
- **Idempotence:** Creating an offer that already exists with the same application and the same endpoints succeeds without change; a different offer under an existing name is rejected, because offers are not updated.
  - *Related error:*
    - **`offer already exists`**: *Trigger:* Creating an offer whose name is already taken by a different offer. *Remediation:* Remove the existing offer or use another name.
- **Consumption:** Consuming an offer validates the consuming user's {ref}`consume access <user-access-offer-consume>`, then creates a proxy application in the consuming model and the records that connect the two models; the usual local application integration follows.
  - *Related error:*
    - **`offer already consumed`**: *Trigger:* A model consumes an offer it already consumes, the proxy application for that offer being registered already. *Remediation:* None; the model's connection to the offer exists.
- **Removal:** Removing an offer stops while the offer still has connections, unless the removal is forced.
  - *Related error:*
    - **`offer has relations`**: *Trigger:* Removing an offer that still has connections without forcing the removal. *Remediation:* Remove the connected relations first, or force the removal.
- **Access:** Creating an offer grants its owner admin access and everyone read access, both as permission records in the controller database (see {ref}`the user access levels <user-access-levels>`).

```{ibnote}
See also: {ref}`Terraform Provider for Juju | Manage offers <tfjuju:manage-offers>`
```

(the-offer-persistence-rules)=
## The offer in the persistence layer

An offer is persisted in the offering model's {ref}`model database <database>` as follows:

- **Offer record:** The identity pair, a `uuid` primary key and the offer's `name`. The offer's URL identifies it across controllers; its name is unique among offers.
  - *Related error:*
    - **`offer not found`**: *Trigger:* A lookup by offer URL or UUID that matches no offer. *Remediation:* Verify the offer URL.
- **Endpoint join record:** Points the offer at the {ref}`application endpoints <application-endpoint>` it publishes.
  - *Rule:* All of an offer's endpoints belong to one application; a schema trigger on the join record keeps every endpoint of one offer on the same application.
- **Connection record:** One record per consumer: the pointer to the consumer's remote relation and the consuming model's offer user.
- **Derived read:** A read joins the offer with its endpoints, the application, the charm, and the connection counts. Total and active connections are derived here, not stored: the read counts an offer's connections and its active connections from the connection records.
- **Consuming side:** In the consuming model's database, the proxy application is a native {ref}`application <application>` record, and a remote-offerer record ties it to the offer (the offer's UUID and URL, the offering controller), with its own status satellite.
- **Users:** The offer's users (who may consume it) live in the controller database as permission records, not in the model (see {ref}`user <user>`).
- **States:** An offer has no life column, no state machine, and no subtypes. The record is static from creation until removal, and its only moving quantity, the number of active connections, is derived from the connections' relations.

**Writers:** The cross-model relation service performs the writes. Offering inserts the offer record with its endpoint records and the controller-side access records; consuming adds the connection record and the consuming side's records; removing deletes the offer with its endpoint, connection, and access records.

## The offer in the execution layer

An offer has no machinery of its own: it is a static record the controller serves; creating, consuming, and removing it are record writes, and what moves across an offer runs in the cross-model relation machinery (see {ref}`cross-model relation <cross-model-relation>`).

(the-offer-watchers)=
### Offer watchers

The offer has no watch surfaces of its own: nothing polls or watches the offer record. What moves across an offer, the remote relation's changes, has its own watchers in the cross-model relation machinery.

Every watcher fires once immediately when it is created, the initial query being the baseline snapshot, and again on each qualifying change. See {ref}`the watcher pattern <watchers>`.
