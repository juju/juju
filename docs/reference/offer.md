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

(offer-access-levels)=
## Offer access levels

Offer levels apply to one offer on the offering controller. {ref}`Offer read access <user-access-offer-read>` permits discovering the offer; {ref}`offer consume access <user-access-offer-consume>` also permits consuming it on the offering side. Consumption separately requires permission to change the consuming model. An {ref}`offer admin <user-access-offer-admin>` can also change access on this offer; that grant alone does not authorize creating or removing offers on the offering model.

A new offer grants its owner offer admin and {ref}`everyone@external <shared-access-for-external-users>` offer read.

(changing-access-to-an-offer)=
### Changing access to an offer

```{ibnote}
See also: {ref}`Juju CLI | Control access to an offer <control-access-to-an-offer>`, [Terraform Provider for Juju | Manage access to an offer](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-offers/#manage-access-to-an-offer)
```

Changing the grant updates an enabled user's direct access on the offering controller. When revocation lowers the direct grant below `consume`, Juju suspends that user's relations to the offer before updating the grant. Users with {ref}`model admin access <user-access-model-admin>` on the offering model or {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller retain their connections. A grant update can fail after suspension. After restoring access, {ref}`resume the relations <manage-relations-to-an-offer>` separately.

**Who can do this:** An {ref}`offer admin <user-access-offer-admin>` on this offer, a {ref}`model admin <user-access-model-admin>` on the offering model or a {ref}`controller superuser <user-access-controller-superuser>` on the offering controller.

**Errors:** The controller can refuse this operation with these errors:

- Ends with `user not found`: The offering controller has no record for the recipient. Check the username and offering controller.
- Ends with `user authentication disabled`: The recipient is disabled on the offering controller. Confirm whether to enable the account before retrying.

