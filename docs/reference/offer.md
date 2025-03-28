(offer)=
# Offer

> See also: {ref}`manage-offers`

In Juju, an **offer** represents an {ref}`application <application>` that has been made available for {ref}`cross-model relations <cross-model-relation>`.

When you are integrating an application with an offer, what you're doing is consume + integrate, where consume = validate that your user has permission to consume the offer + create a local application proxy for the application and integrate is the usual local integrate.

<!--


An *offer* is an application that an administrator makes available to applications residing in other models. The model in which the offer resides is known as the *offering* model.

The application (and model) that utilizes the offer is called the *consuming* application (and consuming model).

Like traditional Juju applications,

- an offer has one or more *endpoints* that denote the features available for that offer.
- a fully-qualified offer endpoint includes the associated offer name:

    `<offer name>:<offer endpoint>`

- a reference to an offer endpoint will often omit the 'offer name' if the context presupposes it.
- an endpoint has an *interface* that satisfies a particular protocol.

-->
