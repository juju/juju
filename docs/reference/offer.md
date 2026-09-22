---
myst:
  html_meta:
    description: "Juju offer reference: applications made available for cross-model relations, enabling integrations across models, controllers, and  clouds."
---

(offer)=
# Offer

```{ibnote}
See also: {ref}`manage-offers`
```

In Juju, an **offer** represents an {ref}`application <application>` that has been made available for {ref}`cross-model relations <cross-model-relation>`.

When you are integrating an application with an offer, what you're doing is consume + integrate, where consume = validate that your user has permission to consume the offer + create a local application proxy for the application and integrate is the usual local integrate.

```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR)
:alt: Two model databases side by side. In the offering model: relation and endpoint records belonging to the offer, offer and offer-connection records, external controller record. In the consuming model: application, relation and endpoint records for the proxy application, remote application record. Arrows follow the foreign keys from each side's records into the shared offer machinery.
```
*The cross-model relation, record by record. The offering side stores
the offer and its connections; the consuming side stores a proxy
application and a remote-application record; both sides agree on the
endpoint and relation records that carry the actual relation data.
Nothing is shared between the two model databases except the offer
URL and credentials.*

