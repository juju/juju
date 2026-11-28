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
:alt: Two arrows labelled “consumed as” lead from one offer to two consuming models. Each contains one highlighted SAAS application. In the upper model two applications integrate with it; in the lower model one does.
```

In Juju, a **SAAS application** is a local stand-in for an {ref}`application <application>` in another {ref}`model <model>`: local applications can integrate with it through the {ref}`endpoints <application-endpoint>` the remote application shares in an {ref}`offer <offer>`. When a {ref}`user <user>` first {ref}`consumes the offer <consuming-an-offer>` in a model, Juju creates one SAAS application for that offer in that model. Local applications use its {ref}`local name <saas-application-names>` to form {ref}`cross-model relations <cross-model-relation>` with the offered application. The SAAS application reports the offered application's {ref}`status <saas-application-status>` until a user {ref}`removes it <removing-a-saas-application>` or the offer goes away.

(saas-application-names)=
## SAAS application names

The first time a user consumes an offer in a model, the SAAS application takes the offer's name unless the user chooses another. Consuming the same offer again in that model does not rename the existing SAAS application. The name must be a valid application name, and must not be the name of another application in the consuming model, including another SAAS application. Giving another name at first consumption lets a user avoid a name already in use.

(saas-application-status)=
## SAAS application status

A SAAS application's status tells the consuming model how the offered application is doing. When the consuming model's controller creates a SAAS application, it sets the status to `unknown`, with the message `waiting for first status update`. From then on, the SAAS application shows the status reported by the offering model, including `error` if the offered application itself is in error.

The SAAS application can also report `error` instead of the offered application's status. If the consuming model's controller cannot connect to the offering model's controller, the message starts with `cannot connect to external controller`. If the offering controller no longer accepts the consuming model's permission to use the offer, the message is the offering controller's refusal.

Model status lists each SAAS application with its status, its message and its offer's URL (in the Juju CLI, the SAAS section of `juju status`). A SAAS application never reports that its offer is gone: when the offer goes away, the consuming model's controller {ref}`removes the SAAS application <removing-a-saas-application>`.

(removing-a-saas-application)=
## Removing a SAAS application

Removing a SAAS application removes both the application and its relations from the consuming model. A user gives the names of one or more SAAS applications and may ask for a forced removal. The consuming model's controller skips any name that does not identify a SAAS application there, without reporting an error.

For each SAAS application, the consuming model's controller marks the SAAS application and each of its relations as dying, schedules their removal, and returns. It deletes the SAAS application once each of those relations has been removed, which waits for the consuming model's units to leave the relation. Until then, model status still shows the SAAS application.

A forced removal removes the relations at once, without waiting for the consuming model's units to leave them. It applies only to the relations that were not already dying when the user asked: a relation that an earlier removal already made dying is not forced, and the SAAS application stays until that relation has been removed.

If the offering model loses the offer (for example, when a user deletes the offered application), the consuming model's controller also removes the SAAS application and its relations. It gives the consuming model's units one minute to leave each relation and then forces the relation's removal.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>` on the consuming model.

**Errors:**

- *Error message:* `permission denied`. *Trigger:* The user does not have model `write` or `admin` on the consuming model. *What to do:* Ask a model admin of the consuming model to remove the SAAS application, or to grant `write`.
- *Error message:* the message given when removals or changes were blocked, or `the operation has been blocked`. *Trigger:* Removals or changes are blocked in the consuming model (in the Juju CLI, with `juju disable-command remove-object` or `juju disable-command all`). *What to do:* If removals should be enabled again, a user with at least model `write` on the consuming model can lift the applicable block. Having `write` does not bypass it.
