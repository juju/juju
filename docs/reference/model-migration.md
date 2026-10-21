---
myst:
  html_meta:
    description: "Juju model migration reference: the records the two controllers keep while a model moves between them, and after it has gone: the export and its phases, the import, the redirect, and the model-side markers."
---

(model-migration)=
# Model migration

```{ibnote}
See also: {ref}`Juju | Migrate a model <migrate-a-model>`
```

A **model migration** is the move of a {ref}`model <model>` from one {ref}`controller <controller>`, the source, to another, the target. The operation — its phases, its rules, and its errors — is described on the {ref}`model <model-migrating>` page; this page holds the records the move leaves behind: the source controller keeps the history of its export attempts and, when the model is gone, a redirect to the target; the target keeps the state of its import; the model's own database keeps the markers of the move. The {ref}`external controller <external-controller>` records name the target.

(model-migration-database-schema)=
## Model migration database schema

```{ggarch}
:file: ../juju.ggarch
:view: Model migration database schema
:caption: The source controller keeps one export record for each attempt to move a model, with the current phase (a phase is the current phase of many exports), the target controller, the phase history (each phase entered once per export), the last status message, one report per machine or unit agent for each phase, the offers to keep for the cleanup, and the user and credentials for reaching the target. The external controller is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Eight boxes of the same size inside a group named Source controller database, each with a filled table mark in the corner. Export is in the middle. Migration phase is above Export (m to 1) and Export phase is to the right of Migration phase, joined to Export (m to 1) and to Migration phase (m to 1). External controller is to the left of Export (m to 1), with Target authorization below it (1 to 1). Status message is to the right of Export (1 to 1), with Exported offer below it (m to 1). Minion report is below Export, joined to Export (m to 1) and to Migration phase (m to 1). External controller has an outlined arrow mark for the record that another page describes.
```

The model migration database schema is the set of records that the two controllers and the model's own database keep about a migration. The records, with the rules that apply to each, are:

- **Export:** One attempt to send the model to a target, on the source controller: the target, the phase the attempt has reached, when it started, and when it last changed. A retry is a new export.
  - *Rules:*
    - A model has at most one export that has not ended (the phases `reap-failed`, `done`, and `abort-done` end it).
- **Migration phase:** The phases of a migration: `quiesce`, `import`, `validation`, `success`, `log-transfer`, `reap`, `reap-failed`, `done`, `abort`, and `abort-done`. The {ref}`model page's figure <model-migrating>` shows which phase can follow which.
  - *Rules:*
    - A phase name appears once.
- **Export phase:** One entry in the history of an export: the phase it entered and when. The current phase of the export is the newest entry.
  - *Rules:*
    - An export enters each phase once.
- **Status message:** The free-form message that the migration master last reported for an export, with the time it was recorded.
  - *Rules:*
    - An export has one status message at a time.
- **Minion report:** One report of a machine or unit {ref}`agent <agent>` of the model, for one phase of an export: whether its work succeeded, and when it reported. The phases `quiesce`, `validation`, and `success` wait for these reports.
  - *Rules:*
    - An export holds one report per agent and phase.
- **Exported offer:** One offer of the model, captured before the model's database is purged, so that the source can remove the offer's permissions after the model is gone.
  - *Rules:*
    - An export holds each offer once.
- **Target authorization:** The user and the credentials the export uses to reach the target controller, with whether the user checks were skipped. The material is per migration; the {ref}`external controller <external-controller>` record it names is shared with the cross-model relations.
  - *Rules:*
    - An export has one target authorization.
- **External controller:** The target the model moves to. Another page owns it: see {ref}`external controller <external-controller>`.
- **Redirect:** Where a model went, on the source controller after the model is gone: the migration that moved it, the target controller, its addresses, its certificate, an alias, when the redirect was made, and when the move completed. The record has no link to the model: the model is gone when the redirect is there. It stays until the model moves again: a later move overwrites the record.
  - *Rules:*
    - A model has at most one redirect.
- **Redirect user:** One user of the model at the moment it moved, with the access level the user had. The snapshot is independent of the live permissions, which are removed with the model.
  - *Rules:*
    - A redirect holds each user once.
- **Import:** The incoming model, on the target controller: the migration it came in, its phase (`importing`, `activating`, or `aborting`), and when the phase last changed. The target holds the model inactive for as long as the import claim exists: the model becomes usable when the import completes and the claim is released.
  - *Rules:*
    - The target keeps one import per model.
- **Imported offer:** One offer that the model consumes, captured so that the target can wire the model's cross-model relations when it activates the model.
  - *Rules:*
    - An import holds each offer once.
- **Imported external model:** One mapping that says which controller hosts a model whose offer the imported model consumes, captured so that the wiring survives a controller restart.
  - *Rules:*
    - An import holds each such model once.
- **Migrating marker:** The marker in the model's own database that the target controller is importing the model. The target creates it when it starts to import the model and removes it when the import completes or aborts. While it is there, the target refuses changes to the model database. The {ref}`agents <agent>` of the model learn that the model is migrating from the {ref}`controller <controller>`, not from the marker.
  - *Rules:*
    - A model has at most one marker.
- **Log transfer progress:** How far the model's logs have been copied to the target, in the model's own database: the time of the last copied entry.
  - *Rules:*
    - A model keeps one progress record.

(model-migration-change-streams)=
## Model migration change streams

Model migration change streams are the notifications that tell the agents of the model and the workers of the controllers that the migration has moved. A unit agent's watcher on the migration flag fires when the export of the model changes phase, and when an import claim for the model appears or disappears. The status watcher tells it each phase, with the addresses of both controllers, so that the agent stops its work in `quiesce` and checks the target in `validation`. The migration master of the source watches the phases of its export and the reports of the agents, so that each phase waits for every agent to answer. The phase history is what the change log records; the status messages are kept out of it, because they change often and would drown the watchers.

(model-migration-operations)=
## Model migration operations

Model migration operations are described on the {ref}`model <model-migrating>` page: a client asks the source controller for a migration; the page states the required access, the rules of every phase, and the errors of the prechecks. The {ref}`external controller <external-controller>` page states how the records of the target controller come to be. The {ref}`lease <lease-and-model-migration>` page states which leases travel.
