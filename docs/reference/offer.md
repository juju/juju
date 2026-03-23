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
