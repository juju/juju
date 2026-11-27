---
myst:
  html_meta:
    description: "Juju SAAS application reference: the local stand-in for an offer, its name and status, and what happens when it is removed."
---

(saas-application)=
# SAAS application

```{ibnote}
See also: {ref}`manage-saas-applications`, {ref}`consume-an-offer`
```

```{ggarch}
:file: ../juju.ggarch
:view: SAAS application across models
:caption: A SAAS application lets applications in your model integrate with an application in another model as if it were in yours. In this example, two models consume the same offer, and each holds one SAAS application for it. In the first consuming model, two applications integrate with that one SAAS application; in the other, one application does.
:alt: A box on the left, Offering model, holds an Offer. Two arrows labelled "consumed as" fan out from the Offer to two boxes stacked on the right. The upper box, Consuming model, holds a SAAS application, in focus, and two arrows labelled "integrated with" run from it to two Applications. The lower box, Another consuming model, holds another SAAS application, in focus, and one arrow labelled "integrated with" runs from it to an Application.
```

In Juju, a **SAAS application** is a local stand-in for an {ref}`offer <offer>`. It is created when a user first {ref}`consumes the offer <consuming-an-offer>` in a model. That consuming model holds at most one SAAS application for the offer. Applications there use its {ref}`local name <saas-application-names>` and the offered endpoints to integrate with the offered application. Each such integration is a {ref}`cross-model relation <cross-model-relation>`. The SAAS application reports the offered application's {ref}`status <saas-application-status>` until a user {ref}`removes it <removing-a-saas-application>` or the offer goes away.

(saas-application-names)=
## SAAS application names

A SAAS application is named after its offer, unless the user who consumes the offer gives another name. The name must be a valid application name, and must not be the name of another application in the consuming model, including another SAAS application. Giving another name lets a user consume an offer whose name an application in the consuming model already has.

(saas-application-status)=
## SAAS application status

A SAAS application's status tells the consuming model how the offered application is doing. When the consuming model's controller creates a SAAS application, it sets the status to `unknown`, with the message `waiting for first status update`. From then on, the offering model reports the status of the offered application, and the SAAS application shows that status. The status becomes `error` in two cases: when the consuming model's controller cannot connect to the offering controller, the controller of the offering model, the message starts with `cannot connect to external controller`; when the offering controller no longer accepts the consuming model's permission to use the offer, the message is the offering controller's refusal.

Model status lists each SAAS application with its status, its message and its offer's URL (in the Juju CLI, the SAAS section of `juju status`). A SAAS application never reports that its offer is gone: when the offer goes away, the consuming model's controller {ref}`removes the SAAS application <removing-a-saas-application>`.

(removing-a-saas-application)=
## Removing a SAAS application

Removing a SAAS application takes it, with its relations, out of the consuming model. A user gives the names of one or more SAAS applications, and may ask for a forced removal. The consuming model's controller skips a name that is not the name of a SAAS application in the consuming model, without an error.

For each SAAS application, the consuming model's controller marks the SAAS application and each of its relations as dying, schedules their removal, and returns. It deletes the SAAS application once each of those relations has been removed, which waits for the consuming model's units to leave the relation. Until then, model status still shows the SAAS application.

A forced removal removes the relations at once, without waiting for the consuming model's units to leave them. It applies only to the relations that were not already dying when the user asked: a relation that an earlier removal already made dying is not forced, and the SAAS application stays until that relation has been removed.

The consuming model's controller also removes a SAAS application, with its relations, when the offering model no longer has the offer, for example because a user removed the offer or deleted the offered application. It gives the consuming model's units one minute to leave each relation, and then forces the relation's removal.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>` on the consuming model.

**Errors:**

- *Error message:* `permission denied`. *Trigger:* The user does not have model `write` or `admin` on the consuming model. *What to do:* Ask a model admin of the consuming model to remove the SAAS application, or to grant `write`.
- *Error message:* the message given when removals or changes were blocked, or `the operation has been blocked`. *Trigger:* Removals or changes are blocked in the consuming model (in the Juju CLI, with `juju disable-command remove-object` or `juju disable-command all`). *What to do:* Ask a user who can change the consuming model to lift the block.
