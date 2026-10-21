---
myst:
  html_meta:
    description: "Juju offer reference: the application endpoints that a model makes available for cross-model relations, the records behind them, the permissions on them, and the operations that create, list, find, show, grant, consume, and remove offers."
---

(offer)=
# Offer

```{ibnote}
See also: {ref}`Juju | Manage offers <manage-offers>`
```

An **offer** is a set of the {ref}`endpoints <application-endpoint>` of one {ref}`application <application>` that a {ref}`model <model>` makes available, so that applications in other models, on other {ref}`controllers <controller>`, and in other {ref}`clouds <cloud>` can integrate with it through a {ref}`cross-model relation <cross-model-relation>`. An offer lives in the model of the offered application. A consuming model gets a local proxy for the offered application, and integrates with that proxy the same way as with any other application.

(offer-data-model)=
## Offer data model

```{ggarch}
:file: ../juju.ggarch
:view: Offer data model
:caption: The model database of the offering model keeps one record for each offer. The record holds the offer's name and an optional description. It also lists the application endpoints that the offer exposes, which the figure leaves out. An offer has many offer connections, one for each relation that consumes it. Each offer connection records the user who made it and belongs to one relation. The controller database keeps one permission for each user of an offer. A permission holds the offer's UUID and an access level, and belongs to one user. A user has many permissions. Because the two databases are separate, no link joins a permission to its offer. A consuming model keeps its own record of each offer that it consumes, which the figure does not draw. The relation and the user are described on their own pages. The figure shows them only for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Five boxes of the same size, each with a filled table mark in the corner. In a group named Model database, Offer is on the left, Offer connection is in the middle, and Relation is on the right. An offer has many offer connections (1 to m). Many offer connections belong to one relation (m to 1). In a group named Controller database below it, Permission is on the left and User is on the right. Many permissions belong to one user (m to 1). No line joins Permission to Offer. Relation and User have an outlined arrow mark for records that other pages describe.
```

The offer data model is the set of records that the offering model keeps about an offer, with the permissions that the {ref}`controller database <database>` keeps for it. The records, with the rules that apply to each, are:

- **Offer:** The offer's name, its description, and the application endpoints that it exposes. A listed endpoint shows its name, interface, role, and limit.
  - *Rules:*
    - An offer exposes each endpoint once.
    - All the endpoints of an offer belong to one application.
- **Offer connection:** One consumer's use of the offer: the offer, the relation that consumes it, and the name of the user who created the connection. The relation is a cross-model relation, described on the {ref}`relation <cross-model-relation>` page.
- **Permission:** The access level of one user on the offer.
  - *Rules:*
    - A user has at most one permission on an offer.
    - The access level of an offer permission is {ref}`read <user-access-offer-read>`, {ref}`consume <user-access-offer-consume>`, or {ref}`admin <user-access-offer-admin>`.
- **Consumed offer:** The record that a consuming model keeps for an offer that it consumes: the offer's UUID and URL, the model that offers it, the controller of that model when it is not this one, and the macaroon (a token that proves the right to use the offer) that the offering side issued.
  - *Rules:*
    - A model consumes an offer once.
- **Relation and user:** The records at the far ends of a connection and a permission. Other pages own them: see {ref}`relation <relation>` and {ref}`user <user>`.

An offer is a row in the table `offer` of the offering model's database. The endpoints it exposes are rows in `offer_endpoint`, each pointing to an application endpoint. Each offer connection is a row in `offer_connection`. The view `v_offer_detail` joins an offer with its application, {ref}`charm <charm>`, endpoints, and connection counts. It also applies the rule that an offer without a description shows the description of the application's charm. A model's consumed offers are rows in `application_remote_offerer`. A permission is a row in the controller database table `permission`, whose column `grant_on` holds the offer's UUID. The view `v_permission_offer` lists the permissions that name an offer. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(offer-change-streams)=
## Offer change streams

Offer change streams are the notifications that tell a consuming model that the status of an offer has changed. A change stream is delivered through a watcher (an object of the controller API that tells its holder when something it follows has changed). The consuming side calls `WatchOfferStatus` on the offering controller with the offer's UUID and the macaroon that the offering side issued. The answer holds the offer's current status (the status, its message, its data, and when it was set). The watcher then fires when the display status of the offered application changes, and when the offer is deleted.

The errors of the call are:

- **`offer "<uuid>" not found`**: *Trigger:* The offering model has no such offer. The client receives it with the code `not found`. *Remediation:* {ref}`Find the offer <offer-finding>` again.
- **The reason that the controller gives**: *Trigger:* The offer UUID cannot be parsed, or the macaroon does not allow access to the offer. *Remediation:* Pass the UUID and the macaroon from the consume details.

(offer-operations)=
## Offer operations

Offer operations are the actions a {ref}`client <client>` can take on offers through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): create, list, find, show, grant access, revoke access, get consume details, consume, and remove. Every operation needs a user who is logged in as a client.

An offer URL names an offer in the form `[<source>:][<qualifier>/]<model>.<offer-name>`. The qualifier is the one that the {ref}`model <model>` carries. It defaults to the caller's. The source names another controller. Showing and getting consume details refuse a URL that has a source. The model name and the offer name must be valid. An offer name follows the rules of an application name. An operation that takes URLs fails a URL with one of these errors:

- **`offer URL has invalid form, must be [<source>:][<qualifier>]<model>.<offername>: "<url>"`**: *Trigger:* The URL does not have that form. *Remediation:* Write the URL in that form.
- **`offer URL is missing model`** and **`offer URL is missing the name`**: *Trigger:* The URL has no model, or no offer name. *Remediation:* Add the missing part.
- **`model name "<name>" not valid`** and **`offer name "<name>" not valid`**: *Trigger:* A name that is not valid. The client receives them with the code `not valid`. *Remediation:* Use a valid name.
- **`model "<qualifier>/<name>": not found`**: *Trigger:* The controller has no such model. The client receives it with the code `not found`. *Remediation:* Check the model's name and qualifier.

The client receives the URL errors in the result for the URL. The model error is also in the result for the URL, except in showing, where it fails the whole call. Listing and finding take no URLs. The model error fails the whole call there.

(offer-creating)=
### Creating offers

```{ibnote}
See also: {ref}`Juju | Create an offer <create-an-offer>`
```

Creating offers makes endpoints of an application available at an offer URL, for the user that the request names as the owner. The call takes a list of offers and accepts exactly one.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the offering model, or {ref}`superuser access <user-access-controller-superuser>` to the controller. When the owner is another user, that user needs the same access.
- *Rules:*
  - The offering model is the one that the request names. When the request names none, it is the model of the call.
  - The owner is the user that the request names. When the request names none, it is the caller. The owner must exist and be enabled.
  - The application name and at least one endpoint are required. The endpoints are the keys of the request's endpoint list. A name that the request gives an endpoint is not stored.
  - The offer's name is the application's name when the request has no name.
  - The offer's description is stored when the request has one.
  - The application must exist and must not be dead. Its endpoints must exist and must have the global scope (the scope of an ordinary relation). An endpoint with the container scope (the scope of a subordinate relation) cannot be offered.
  - An offer name is unique in the model. Creating an offer with the name, application, and endpoints of an existing offer succeeds and changes nothing.
  - A new offer gives its owner admin access and gives the special user `everyone@external` (the user that stands for every external user) read access.
  - If storing the permissions fails, the offer is deleted again.
- *Errors:* The client receives each in the result, with a code where one is given:
  - **`expected exactly one offer, got <n>`**: *Trigger:* The request holds no offer, or more than one. It fails the whole call and has no code. *Remediation:* Send one offer in each call.
  - **`checking user "<caller tag>" has admin permission on model "<uuid>": permission denied`**: *Trigger:* The caller lacks the access. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the model or a superuser.
  - **`checking user "<owner tag>" has admin permission on model "<uuid>": permission denied`**: *Trigger:* The owner is another user who lacks the access. The client receives it with the code `not valid`. *Remediation:* Grant the owner admin access to the model, or name another owner.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid model tag`** (or **`user tag`**): *Trigger:* The model tag or the owner tag is malformed, or of another kind. *Remediation:* Use a tag of the right kind.
  - **`application name cannot be empty`**, **`owner name cannot be empty`**, and **`endpoints cannot be empty`**: *Trigger:* The part is missing. The client receives them with the code `not valid`. *Remediation:* Provide it.
  - **`offer "<name>" already exists, updating offers is not supported`**: *Trigger:* An offer with this name exists and offers other endpoints, or another application. The client receives it with the code `bad request`. *Remediation:* {ref}`Remove the offer <offer-removing>` first, or choose another name.
  - **`application "<name>" not found in model "<uuid>"`**: *Trigger:* The model has no such application. The client receives it with the code `not found`. *Remediation:* Name an application of the model.
  - **`creating offer "<name>": application is dead`**: *Trigger:* The application is dead. *Remediation:* Offer another application.
  - **`creating offer "<name>": "<endpoints>": endpoint not found`** and **`creating offer "<name>": not all endpoints found "<endpoints>" for application "<uuid>"`**: *Trigger:* An endpoint is not one of the application's endpoints. *Remediation:* Name endpoints of the application's charm.
  - **`creating offer "<name>": can only offer endpoints with global scope, provided scope "container"`**: *Trigger:* An endpoint has the container scope. *Remediation:* Offer only endpoints with the global scope.
  - **`creating offer: "<owner>": user not found`** and **`creating offer: "<owner>": user authentication disabled`**: *Trigger:* The owner does not exist, or is disabled. *Remediation:* Name an existing, enabled user.

(offer-listing)=
### Listing offers

```{ibnote}
See also: {ref}`Juju | View an offer's details <view-an-offers-details>`
```

Listing offers returns the details of the offers that match filters, in the models that the filters name, together with their connections.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to each model that a filter names, or {ref}`superuser access <user-access-controller-superuser>` to the controller. Offer admin access is not enough.
- *Rules:*
  - At least one filter is required. Each filter names a model. The qualifier of that model defaults to the caller's.
  - A filter can hold an offer name, an application name, a description, endpoint terms, allowed consumers, and connected users. The offer name and the description match when they contain the given text, ignoring case. The application name matches exactly. An endpoint term (a name, an interface, and a role, each optional) is its own alternative, so an offer matches when any endpoint term matches, or when the remaining terms together match. A filter that names only a model matches every offer in it.
  - An allowed consumer is a user tag. A filter that names allowed consumers also matches the offers on which any of them has consume or admin access. A connected user is a user tag that has no effect.
  - An offer name in a filter can be written as a quoted literal (`^name$`). Any other pattern is refused.
  - Each result holds the offer's URL, name, UUID, source model, description, endpoints, application name, {ref}`charm <charm>` URL, users with their access, and connections.
  - The users are every user with a permission on the offer except `everyone@external` (the special user that stands for every external user). When the caller is not among them, the caller is added as admin.
  - A connection holds the source model, the relation ID, the user, the endpoint, the status with its message and time, and the ingress subnets (the subnets from which the remote side connects).
  - The offers come in the order of their models' UUIDs.
- *Errors:* The client receives each as the error of the whole call:
  - **`permission denied`**: *Trigger:* The caller is not an admin of a named model. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the model or a superuser.
  - **`at least one offer filter is required`**: *Trigger:* The request has no filter. *Remediation:* Add a filter that names a model.
  - **`application offer filter must specify a model name`**: *Trigger:* A filter names no model. *Remediation:* Name the model.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid user tag`**: *Trigger:* A user tag of a filter is malformed, or of another kind. *Remediation:* Use a user tag.
  - **`unescaping offer name: not a quoted meta`**: *Trigger:* The offer name is a pattern. *Remediation:* Write the name as a literal.

(offer-finding)=
### Finding offers

```{ibnote}
See also: {ref}`Juju | Find an offer to use <find-an-offer-to-use>`
```

Finding offers returns the offers that match filters and that the caller can see. A caller sees an offer when the caller has read access to its model or access to the offer at the read level or above.

The operation's required access, rules, and errors are:

- *Required access:* A login only. An offer that the caller may not see is left out of the answer, with no error.
- *Rules:*
  - The filters and the results are those of {ref}`listing <offer-listing>`.
  - A request with exactly one filter and no model name queries every model on the controller.
  - An admin of a model sees every user of each offer and the offer's connections. A caller who is not an admin sees only the caller's own access in the users and sees no connections.
- *Errors:* The client receives the filter errors of {ref}`listing <offer-listing>` as the error of the whole call.

(offer-showing)=
### Showing offers

```{ibnote}
See also: {ref}`Juju | View an offer's details <view-an-offers-details>`
```

Showing offers returns the details of the offers that the given URLs name.

The operation's required access, rules, and errors are:

- *Required access:* The same as {ref}`finding <offer-finding>`.
- *Rules:*
  - The call takes a list of URLs. Each URL gets its own result, an offer or an error.
  - The URL must be local and must not name an endpoint.
  - A URL with no matching offer that the caller can see gives a not found error.
- *Errors:* The client receives each in the result for the URL:
  - **`saas application "<url>" shouldn't include endpoint`** and **`query for non-local application offers`**: *Trigger:* The URL names an endpoint (`:<endpoint>`), or has a source (`<source>:`). The client receives them with the code `not supported`. *Remediation:* Use the offer's local URL.
  - **`application offer "<url>" not found`**: *Trigger:* No offer matches, or the caller may not see it. The client receives it with the code `not found`. *Remediation:* {ref}`Find the offer <offer-finding>`.

(offer-access)=
### Granting and revoking offer access

```{ibnote}
See also: {ref}`Juju | Control access to an offer <control-access-to-an-offer>`
```

Granting and revoking offer access changes the access level of a user on an offer. The call takes a list of changes, each with an offer URL, a user, an access level (read, consume, or admin), and an action (`grant` or `revoke`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller, {ref}`admin access <user-access-model-admin>` to the offering model, or {ref}`admin access <user-access-offer-admin>` to the offer.
- *Rules:*
  - A grant sets the user's permission to the given level, and creates the permission when the user has none. A user who already has that level or a higher one gets an error.
  - A revoke sets the user's access to the level below the revoked one: revoking admin leaves consume, revoking consume leaves read, and revoking read removes the permission.
  - A revoke that leaves a user below consume suspends every relation to the offer that the user created, with the reason `offer access revoked`. A controller superuser or an admin of the model keeps consume access, so those relations stay active.
  - The user must exist and be enabled.
  - The suspension happens before the permission is looked up, so a revoke that fails may already have suspended the user's relations. To keep them active, resume them.
- *Errors:* The client receives each in the result for the change:
  - **`permission denied`**: *Trigger:* The caller has none of the three accesses. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the offer or the model, or a superuser.
  - **`offer "<url>" not found`**: *Trigger:* The model has no such offer. The client receives it with the code `not found`. *Remediation:* {ref}`Find the offer <offer-finding>`.
  - **`unknown action "<action>"`**: *Trigger:* The action is neither `grant` nor `revoke`. *Remediation:* Use one of the two.
  - **`"<access>" offer access not valid`**: *Trigger:* The level is not read, consume, or admin. The client receives it with the code `not valid`. *Remediation:* Use one of the three.
  - **`updating offer permission for "<user>" on "<offer uuid>": looking up user "<user>": "<user>": user not found`** (or **`user authentication disabled`**): *Trigger:* The user does not exist, or is disabled. *Remediation:* Name an existing, enabled user.
  - **`updating offer permission for "<user>" on "<offer uuid>": user "<user>" already has "<access>" access or greater`**: *Trigger:* A grant of a level that the user has. *Remediation:* Continue; the user has the access.
  - **`updating offer permission for "<user>" on "<offer uuid>": offer permission for "<user>" on "<offer uuid>" permission not found`**: *Trigger:* A revoke from a user with no permission on the offer. *Remediation:* Continue; the user has no access to revoke.
  - The URL errors and the model error of the {ref}`opener <offer-operations>`. For the user, the tag errors of {ref}`creating <offer-creating>`.

(offer-consume-details)=
### Getting consume details

```{ibnote}
See also: {ref}`Juju | Integrate with an offer <integrate-with-an-offer>`
```

Getting consume details returns what a consuming model needs to use an offer: the offer, how to reach the offering controller, and a macaroon (a token that proves the right to use the offer) for one user.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the offering model or {ref}`superuser access <user-access-controller-superuser>` to the controller, or {ref}`consume access <user-access-offer-consume>` (or higher) to the offer. Asking for another user's details needs superuser access.
- *Rules:*
  - The user is the caller, or the user that the request names.
  - The call takes a list of URLs. Each URL gets its own result.
  - The URL must be local and must not name an endpoint.
  - A superuser with no access of their own to the offer can still get the details. JAAS, a service that sits in front of controllers and checks the user's permissions itself, relies on this.
  - The result holds the offer's UUID, URL, name, source model, and endpoints (name, interface, role, and limit). It holds the offering controller's tag, API addresses, and certificate authority certificate (the certificate that a consumer trusts when it connects to the controller). It holds a macaroon for the user, in the macaroon format that the request names.
  - The controller's public DNS address, when configured, comes first among the addresses.
- *Errors:* The client receives each in the result for the URL. The permission error for another user's details is the error of the whole call:
  - **`unable parse offer URL "<url>": <reason>`**: *Trigger:* The URL cannot be parsed. The client receives it with the code `bad request`. *Remediation:* Write the URL in the form of the {ref}`opener <offer-operations>`.
  - **`saas application "<url>" shouldn't include endpoint`** and **`query for non-local application offers`**: *Trigger:* As for {ref}`showing <offer-showing>`. The client receives them with the code `not supported`. *Remediation:* Use the offer's local URL.
  - **`application offer "<url>" not found`**: *Trigger:* The model has no such offer. The client receives it with the code `not found`. *Remediation:* {ref}`Find the offer <offer-finding>`.
  - **`permission denied`**: *Trigger:* The caller has no access to the offer, or asked for another user's details without superuser access. *Remediation:* Ask an admin of the offer to {ref}`grant consume access <offer-access>`.

(offer-consuming)=
### Consuming offers

```{ibnote}
See also: {ref}`Juju | Integrate with an offer <integrate-with-an-offer>`
```

Consuming offers adds a remote application to the consuming model: a local proxy for the offered application. The proxy has the offer's endpoints and takes the name that the request gives, or the offer's name. The operation creates no relation.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Write access <user-access-model-write>` to the consuming model. The change block (an operator setting that makes the controller refuse operations that change the model; a refused operation fails with an "operation blocked" error that carries the operator's message) must not be on.
- *Rules:*
  - The call takes a list of offers, each with the details that {ref}`getting consume details <offer-consume-details>` returned.
  - The model stores the offering controller's details, unless that controller is this one.
  - The role of an endpoint is provider or requirer. A peer endpoint is refused.
  - The name must be a valid application name.
  - Consuming an offer that the model already consumes succeeds and keeps the existing remote application, whatever name the request gives. The model's record of the offering controller is updated first.
- *Errors:* The client receives each in the result for the offer, with a code where one is given. The access error and the block fail the whole call:
  - **`parsing source model tag: <reason>`**, **`saving external controller info: parsing controller tag "<tag>": <reason>`**, and **`parsing offer URL: <reason>`** (when the request gives no name): *Trigger:* A malformed tag, or an offer URL that cannot be parsed. The client receives them with the code `bad request`. *Remediation:* Pass the tags and the URL from the consume details.
  - **`parsing offer UUID: <reason>`**, **`parsing offer URL: <reason>`**, and **`saving external controller info: updating external controller "<uuid>": <reason>`**: *Trigger:* The offer's UUID or URL in the details cannot be parsed, or the controller's details cannot be stored. They have no code. *Remediation:* Pass the details as the consume details returned them.
  - **`parsing role for endpoint "<name>": peer relations cannot be used in cross model`** and **`parsing role for endpoint "<name>": endpoint role must be "provider" or "requirer", got "<role>"`**: *Trigger:* An endpoint of another role. The client receives them with the code `bad request`. *Remediation:* Pass the endpoints as the consume details returned them.
  - **`application "<name>" already exists`**: *Trigger:* The model has an application with that name. The client receives it with the code `already exists`. *Remediation:* Give the offer another name.
  - **`saving remote application offerer "<name>": application name not valid`**: *Trigger:* The name is not a valid application name. *Remediation:* Use a valid name.
  - **`saving remote application offerer "<name>": macaroon cannot be nil`**: *Trigger:* The request has no macaroon. The client receives it with the code `not valid`. *Remediation:* Pass the macaroon from the consume details.
  - **`permission denied`**: *Trigger:* The caller has no write access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the model to grant write access.
  - **The operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Ask the operator to lift the block.

(offer-removing)=
### Removing offers

```{ibnote}
See also: {ref}`Juju | Remove an offer <remove-an-offer>`
```

Removing offers deletes offers from the offering model at once, with the permissions on them.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Admin access <user-access-model-admin>` to the offering model or {ref}`superuser access <user-access-controller-superuser>` to the controller. Offer admin access is not enough.
- *Rules:*
  - The call takes a list of URLs and a force option. Each URL gets its own result.
  - An offer that has connections is refused, unless the request forces the removal.
  - A forced removal also deletes the relations of the connections, with their relation units (the records of each unit's membership in a relation).
  - Removing an offer deletes every permission on it. A failure of that step does not fail the call.
  - Removing an application also deletes the offers of its endpoints.
- *Errors:* The client receives each in the result for the URL, with a code where one is given:
  - **`permission denied`**: *Trigger:* The caller is not an admin of the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the model or a superuser.
  - **`"<name>": offer not found`**: *Trigger:* The model has no offer with that name. It has no code. *Remediation:* {ref}`Find the offer <offer-finding>`.
  - **`deleting offer "<uuid>": cannot delete offer {"<uuid>"}, it has <n> connections`**: *Trigger:* The offer has connections and the request does not force the removal. It has no code. *Remediation:* {ref}`Remove the relations <manage-offers>` first, or force the removal.
  - The URL errors and the model error of the {ref}`opener <offer-operations>`.

## Offers and model migration

When a {ref}`model <model>` is migrated to another controller, its offers move with it and keep their UUIDs. The target controller writes the permissions on the offers together with the model's other permissions. The offers that the model consumes move too. See {ref}`Migrating models <model-migrating>`.
