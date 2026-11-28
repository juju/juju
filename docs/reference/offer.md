---
myst:
  html_meta:
    description: "Juju offer reference: what an offer is, its URL, access and connections, and what the controller accepts and refuses when offers are created, viewed, shared, listed, removed, found and consumed."
---

(offer)=
# Offer

```{ibnote}
See also: {ref}`manage-offers`
```

```{ggarch}
:file: ../juju.ggarch
:view: Offer across models
:caption: An offer lets applications in other models use only the endpoints you choose to share. Here the offering model shares endpoints A and B of one application. In the consuming model, an application integrates with the SAAS application for that offer. That relation counts as one connection to the offer. Endpoint C is not accessible through it.
:alt: The offering model contains an application with endpoints A, B and C. Only A and B have arrows to the offer. An arrow labelled “consumed as” leads from the offer to a SAAS application in the consuming model. A second arrow connects the SAAS application to an application in that model.
```

In Juju, an **offer** is a set of {ref}`endpoints <application-endpoint>` that one {ref}`application <application>` shares outside its {ref}`model <model>` (the offering model). Applications in other models can use those endpoints to form a {ref}`cross-model relation <cross-model-relation>`. A {ref}`user <user>` can {ref}`consume the offer <consuming-an-offer>` in another model (the consuming model). Juju creates one {ref}`SAAS application <saas-application>` for the offer the first time it is consumed in that model. Each relation to that SAAS application counts as one {ref}`connection <offer-connections>` to the offer.

Users of the offering model's {ref}`controller <controller>` can hold {ref}`offer access <offer-access>` separately from their model access. Only a {ref}`model admin <user-access-model-admin>` on the offering model or a {ref}`controller superuser <user-access-controller-superuser>` on its controller can {ref}`create <creating-an-offer>` or {ref}`remove <removing-an-offer>` offers. Either can {ref}`grant offer access <changing-access-to-an-offer>`, as can an {ref}`offer admin <user-access-offer-admin>`. A user with {ref}`model read access <user-access-model-read>` on the offering model can {ref}`find every offer <finding-offers>` in it; a model admin there can also {ref}`list its offers' connections <listing-offers-and-their-connections>`.

(offer-urls)=
## Offer URLs

An offer URL has the form `<qualifier>/<model>.<offer>`. The qualifier and the model name identify the offering model; the qualifier is set when the model is created, and is usually the name of the user who created it. The offer name identifies the offer in the offering model. A client that works with several controllers may let a user put a controller name and a colon in front of the URL to choose the controller it sends the request to, as the Juju CLI does (`prod:alice/default.hosted-mysql`); the controller name is not part of the offer's URL.

(offer-access)=
## Offer access

A user's access to an offer is one of three levels, in increasing order: {ref}`read <user-access-offer-read>`, {ref}`consume <user-access-offer-consume>` and {ref}`admin <user-access-offer-admin>`. The offering controller holds offer access separately from model access, and some access levels on the offering model and the offering controller also allow offer operations.

Every offer has an owner: the user who created it, or another user that they named as the owner when they {ref}`created it <creating-an-offer>`. When the offering controller creates an offer, it grants the owner `admin` on the offer, and grants `read` to the user `everyone@external`. The `everyone@external` user stands for every user authenticated by an external identity provider, so this `read` grant reaches only those users. Any other user, including every local user, has only the offer access that a user grants them by name.

A user's access also decides which offers they see when they view or find offers, and whose access each offer lists:

- A user with {ref}`model admin access <user-access-model-admin>` on the offering model, or with {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller, sees every offer in the offering model and the access of every user to each.
- A user with at least {ref}`model read access <user-access-model-read>` on the offering model sees every offer in it, whatever their access to the offer, and only their own offer access.
- Any other user sees only the offers on which they have at least {ref}`offer read access <user-access-offer-read>`, and only their own offer access.

No list of users includes the `everyone@external` grant.

(offer-connections)=
## Offer connections

An offer's connections are the relations that applications in consuming models have with it, one connection for each relation. A connection records the consuming model, the user who made the relation, the endpoint, the relation and its status. Its ingress subnets are the source networks that the consuming side reports for traffic to the offering side over that relation.

While any connection remains, even a suspended one, ordinary removal of the offer is blocked. To remove the offer without forcing removal, first remove the relations to it.

(managing-an-offer-on-the-offering-side)=
## Managing an offer on the offering side

(creating-an-offer)=
### Creating an offer

Creating an offer shares endpoints of an application in the offering model with other models. A user gives the application and at least one of its endpoints, and may give the offer a name; an offer without a name is named after the application. Every endpoint must have global scope. The offering controller refuses an endpoint with container scope, which a subordinate application uses to attach to its principal.

The user who creates the offer owns it, unless they name another user as the owner. That user must also have model `admin` on the offering model or controller `superuser` on the offering controller.

An offer cannot be updated. If the offering model already has an offer with the requested name, the offering controller compares the two by application and endpoints. If they match, the request succeeds and changes nothing, even if other details, such as the owner, differ; if they do not, the offering controller refuses the request. To change an offer, a user removes it and creates it again.

**Who can do this:** A user with {ref}`model admin access <user-access-model-admin>` on the offering model, or with {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.

**Errors:**

- *Error message:* `checking user "user-<name>" has admin permission on model "<model-uuid>": permission denied`. *Trigger:* The user who creates the offer, or the user named as its owner, has neither model `admin` on the offering model nor controller `superuser` on the offering controller, or the named owner does not exist. *What to do:* If the owner's name is wrong, correct it, or omit it to own the offer yourself. If the creator or the intended owner lacks access, ask a model admin of the offering model to grant that user model `admin` or choose a creator or owner who already has the required access.
- *Error message:* `application name cannot be empty`. *Trigger:* The user gives no application. *What to do:* Give the name of an application in the offering model.
- *Error message:* `endpoints cannot be empty`. *Trigger:* The user gives no endpoint. *What to do:* Give at least one of the application's endpoints.
- *Error message:* `application "<application>" not found in model "<model-uuid>"`. *Trigger:* The offering model has no application with that name. *What to do:* Give the name of an application in the offering model.
- *Error message:* ends with `endpoint not found`, or with `not all endpoints found "<endpoints>" for application "<application-uuid>"`. *Trigger:* An endpoint is not one of the application's endpoints. *What to do:* Give only endpoints that the application has.
- *Error message:* ends with `can only offer endpoints with global scope, provided scope "container"`. *Trigger:* An endpoint has container scope. *What to do:* Offer only endpoints with global scope.
- *Error message:* `offer "<offer>" already exists, updating offers is not supported`. *Trigger:* The offering model has an offer with that name for another application or other endpoints. *What to do:* Use another name, or remove the existing offer and create it again.

(viewing-an-offer)=
### Viewing an offer

```{ibnote}
See also: {ref}`View one offer's details with the Juju CLI <view-an-offers-details>`
```

Viewing an offer by its exact {ref}`URL <offer-urls>` returns its description, offered endpoints and the access entries visible to the caller. Each endpoint has a name, interface and role. A user with at least model `read` on the offering model can view the offer without a grant on the offer itself. The {ref}`visibility rules <offer-access>` also determine whose access entries the caller sees.

The offering controller returns the offer's own description if it has one. Otherwise it returns the description from the offered application's charm metadata, not the charm's README.

**Who can do this:** Any user who can log in to the offering controller.

**Errors:**

- *Error message:* `application offer "<qualifier>/<model>.<offer>" not found`. *Trigger:* The offer does not exist, or the user cannot see it. *What to do:* Check the offer URL, or ask the offer's admin for `read` access.
- *Error message:* `saas application "<url>" shouldn't include endpoint`. *Trigger:* The URL identifies an endpoint rather than the offer alone. *What to do:* Omit the endpoint from the offer URL.

(changing-access-to-an-offer)=
### Changing access to an offer

Changing access to an offer grants a user an offer access level or revokes one. A user gives the other user's name, the level and the offer URL; the other user must be a user of the offering controller.

If a user relies on an offer grant to consume the offer, revoking `consume` or `read` takes away that ability. Before lowering the user's access, the offering controller suspends each of their relations to the offer with the message `offer access revoked`. If the user can still consume the offer as a model `admin` on the offering model or a controller `superuser` on the offering controller, their relations stay as they are. Granting access again does not resume a suspended relation; each one stays suspended until a user resumes it.

**Who can do this:** A user with {ref}`offer admin access <user-access-offer-admin>` on the offer, with {ref}`model admin access <user-access-model-admin>` on the offering model, or with {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.

**Errors:**

- *Error message:* `offer "<qualifier>/<model>.<offer>" not found`. *Trigger:* The offer does not exist. *What to do:* Check the offer URL.
- *Error message:* `permission denied`. *Trigger:* The user has neither offer `admin` on the offer, model `admin` on the offering model, nor controller `superuser` on the offering controller. *What to do:* Ask the offer's admin to change the access.
- *Error message:* ends with `user not found`. *Trigger:* The offering controller has no user with that name. *What to do:* Ask a controller admin of the offering controller to add the user, or give the name of a user it has.
- *Error message:* ends with `user authentication disabled`. *Trigger:* The user is disabled on the offering controller. *What to do:* Ask a controller admin of the offering controller to enable the user.
- *Error message:* ends with `access or greater`. *Trigger:* A grant gives a level that the user already has, or a lower one. *What to do:* If the user needs at least the requested level, no change is needed. To lower their access, revoke the access they no longer need.

(listing-offers-and-their-connections)=
### Listing offers and their connections

```{ibnote}
See also: {ref}`Inspect offer connections with the Juju CLI <inspect-relations-to-an-offer>`
```

Listing offers shows the offers of one or more offering models with their {ref}`connections <offer-connections>`. A user gives one or more filters, and each filter must name an offering model; the list covers the models that the filters name.

An offer name matches names containing that text. An application name must match exactly. When both are given, the offer must match both. An interface match or a user's explicit `consume` or `admin` grant adds offers to those results rather than narrowing them. On a 4.1 controller, specifying a connected user does not restrict the results.

If no offer in the selected models matches, the list is empty. To look up one known offer instead, {ref}`view it by URL <viewing-an-offer>`.

**Who can do this:** A user with {ref}`model admin access <user-access-model-admin>` on each offering model that the filters name, or with {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.

**Errors:**

- *Error message:* `permission denied`. *Trigger:* The user has neither model `admin` on an offering model that the filters name nor controller `superuser` on the offering controller, whatever their access to its offers. *What to do:* Ask a model admin of that offering model to list the offers.
- *Error message:* `application offer filter must specify a model name`. *Trigger:* A filter names no model. *What to do:* Name an offering model in each filter.

(removing-an-offer)=
### Removing an offer

```{ggarch}
:file: ../juju.ggarch
:view: Offer removal
:caption: Removing an offer deletes it from the offering model before the consuming model finishes cleanup. Here an admin has already removed the offer's relations. The consuming model still has the SAAS application until its controller learns that the offer is gone and removes the application. One controller can manage both models.
:alt: A sequence with four lifelines: Offering model's admin, Client, Offering model's controller and Consuming model's controller. The admin runs juju remove-offer; the client asks the offering model's controller to remove the offer; that controller deletes the offer and returns "removed"; the client returns "done" to the admin. Only after that, a watcher tells the consuming model's controller that the offer is terminated, and it removes the SAAS application.
```

Removing an offer deletes it from the offering model at once. A user gives the offer URL and may ask for a forced removal. Forced removal also removes the offer's {ref}`connections <offer-connections>` and the relations to the offer in the offering model. A consuming model keeps both its SAAS application and that application's relations until its controller detects that the offer is gone and {ref}`removes them <removing-a-saas-application>`. A consuming model can therefore still show the SAAS application after a successful removal.

If a request names several offers, the offering controller reports success or failure for each. Failure to remove one does not undo the removal of another.

When the offering controller deletes an application, it also deletes the application's offers.

**Who can do this:** A user with {ref}`model admin access <user-access-model-admin>` on the offering model, or with {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller.

**Errors:**

- *Error message:* `permission denied`. *Trigger:* The user has neither model `admin` on the offering model nor controller `superuser` on the offering controller, whatever their access to the offer. *What to do:* Ask a model admin of the offering model to remove the offer.
- *Error message:* ends with `offer not found`. *Trigger:* The offering model has no offer with that name. *What to do:* Check the offer URL.
- *Error message:* ends with `it has <count> connections`. *Trigger:* The offer has {ref}`connections <offer-connections>` and the removal is not forced. *What to do:* Remove the relations to the offer, then remove the offer.

(using-an-offer-on-the-consuming-side)=
## Using an offer on the consuming side

(finding-offers)=
### Finding offers

```{ibnote}
See also: {ref}`Find an offer with the Juju CLI <find-an-offer-to-use>`
```

Finding offers shows the offers that match a filter on their offering model, name, application or endpoints. A user gives one or more filters. A single filter that names no model searches every model on the controller that receives the request. Results include only {ref}`visible offers <offer-access>`. A user with at least model `read` on an offering model sees every matching offer in it, even without a grant on the offer itself.

Within the models searched, an offer name matches names containing that text. A search with both a name and an interface returns offers that match either.

If no visible offer matches, the result is empty. Use the {ref}`URL of a matching offer <offer-urls>` to {ref}`view that offer's details <viewing-an-offer>`.

**Who can do this:** Any user who can log in to the controller that receives the request.

**Errors:**

- *Error message:* `application offer filter must specify a model name`. *Trigger:* The user gives more than one filter, and one of them names no model. *What to do:* Name an offering model in each filter, or give a single filter.

(consuming-an-offer)=
### Consuming an offer

```{ibnote}
See also: {ref}`Juju CLI: Consume an offer <consume-an-offer>`
```

```{ggarch}
:file: ../juju.ggarch
:view: Consuming an offer
:caption: The offering model's controller authorizes access to the offer. On first use, the consuming model's controller creates the SAAS application in that model. The same controller can serve both models.
:alt: A user runs juju consume. The client gets the offer details and permission from the offering model's controller, then passes them to the consuming model's controller. That controller creates a SAAS application with status unknown and returns success to the client.
```

When a user consumes an offer, the offering model's controller first authorizes access to the offer. On first use in the consuming model, the consuming model's controller creates a {ref}`SAAS application <saas-application>` with the offer's endpoints and the chosen {ref}`local name <saas-application-names>`. If the models are on different controllers, the consuming model's controller also records how to reach the offering model's controller. Applications in the consuming model can then integrate with the SAAS application in a {ref}`cross-model relation <cross-model-relation>`.

If the consuming model already has a SAAS application for the offer, consuming it again succeeds without creating another application or changing the existing name. This also applies while the existing SAAS application is being removed.

**Who can do this:** The user needs access on **both** sides:

- On the offering side: at least {ref}`offer consume access <user-access-offer-consume>` on the offer, {ref}`model admin access <user-access-model-admin>` on the offering model, or {ref}`controller superuser access <user-access-controller-superuser>` on the offering controller. Model `read` or `write` on the offering model alone is not enough.
- On the consuming side: at least {ref}`model write access <user-access-model-write>` on the consuming model.

**Errors:**

- *Error message:* `application offer "<qualifier>/<model>.<offer>" not found`. *Trigger:* The offer does not exist. *What to do:* Check the offer URL.
- *Error message:* `permission denied`. *Trigger:* The offering controller refuses access to the offer, or the consuming controller refuses the change to the consuming model. *What to do:* The message alone does not identify the side that refused. Check both access requirements above. If offering-side access is missing, ask an offer admin to grant `consume` on the offer. If consuming-side access is missing, ask a model admin of the consuming model to grant `write`.
- *Error message:* the message given when changes were blocked, or `the operation has been blocked`. *Trigger:* Changes are blocked in the consuming model. *What to do:* If changes should be enabled again, a user with at least model `write` on the consuming model can lift the block. Having `write` does not bypass it.
- *Error message:* ends with `application name not valid`. *Trigger:* The chosen local name is not a valid application name. *What to do:* Choose an unused application name such as `remote-db`.
- *Error message:* `application "<name>" already exists`. *Trigger:* Another application in the consuming model already has the requested name. *What to do:* Choose an unused name.
