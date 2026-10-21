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

(model-migration-data-model)=
## Model migration data model

```{ggarch}
:file: ../juju.ggarch
:view: Model migration data model
:caption: The source controller keeps one export record for each attempt to move a model, with the current phase (a phase is the current phase of many exports), the target controller, the phase history (each phase entered once per export), the last status message, one report per machine or unit agent for each phase, the offers to keep for the cleanup, and the user and credentials for reaching the target. The external controller is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Eight boxes of the same size inside a group named Source controller database, each with a filled table mark in the corner. Export is in the middle. Migration phase is above it (m to 1) and External controller is to its left (m to 1). Export phase is above Export phase on the right, joined to both Export (m to 1) and Migration phase. Status message is to the right of Export (1 to 1). Minion report is below Export, joined to both Export and Migration phase. Exported offer and Target authorization are below External controller, joined to Export.
```

The model migration data model is the set of records that the two controllers and the model's own database keep about a migration. The records, with the rules that apply to each, are:

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
- **Minion report:** One report of a machine or unit {ref}`agent <agent>` of the model, for one phase of an export: whether its work succeeded, and when it reported. The phase `quiesce` and the phase `validation` wait for these reports.
  - *Rules:*
    - An export holds one report per agent and phase.
- **Exported offer:** One offer of the model, captured before the model's database is purged, so that the source can remove the offer's permissions after the model is gone.
  - *Rules:*
    - An export holds each offer once.
- **Target authorization:** The user and the credentials the export uses to reach the target controller, with whether the user checks were skipped. The material is per migration; the {ref}`external controller <external-controller>` record it names is shared with the cross-model relations.
  - *Rules:*
    - An export has one target authorization.
- **External controller:** The target the model moves to. Another page owns it: see {ref}`external controller <external-controller>`.
- **Redirect:** Where a model went, on the source controller after the model is gone: the migration that moved it, the target controller, its addresses, its certificate, an alias, when the redirect was made, and when the move completed. The record has no link to the model: the model is gone when the redirect is there. It stays until the source forgets the model.
  - *Rules:*
    - A model has at most one redirect.
- **Redirect user:** One user of the model at the moment it moved, with the access level the user had. The snapshot is independent of the live permissions, which are removed with the model.
  - *Rules:*
    - A redirect holds each user once.
- **Import:** The incoming model, on the target controller: the migration it came in, its phase (`importing`, `activating`, or `aborting`), and when the phase last changed. The target holds the model inactive while the phase is `importing`.
  - *Rules:*
    - The target keeps one import per model.
- **Imported offer:** One offer that the model consumes, captured so that the target can wire the model's cross-model relations when it activates the model.
  - *Rules:*
    - An import holds each offer once.
- **Imported external model:** One mapping that says which controller hosts a model whose offer the imported model consumes, captured so that the wiring survives a controller restart.
  - *Rules:*
    - An import holds each such model once.
- **Migrating marker:** The marker in the model's own database that the model is being migrated. It exists while a migration of the model runs. Its presence is what the {ref}`agents <agent>` of the model watch to stop their work.
  - *Rules:*
    - A model has at most one marker.
- **Log transfer progress:** How far the model's logs have been copied to the target, in the model's own database: the time of the last copied entry.
  - *Rules:*
    - A model keeps one progress record.

An export is a row in the table `model_migration_export`; its history in `model_migration_export_phase`; its status message in `model_migration_export_status`; its minion reports in `model_migration_export_minion_sync`; its offers in `model_migration_export_offer`; its target authorization in `model_migration_export_target_auth`. The phases are rows in the lookup table `model_migration_phase`. A redirect is a row in `model_migration_redirect`. Its users are rows in `model_migration_redirect_user`. An import is a row in `model_migration_import`, with its phase in the lookup table `model_migration_import_phase_type`, its offers in `model_migration_import_offer`, and its external models in `model_migration_import_external_controller_model`. All of these are in the controller database; the migrating marker is a row in `model_migrating` and the log transfer progress in `log_transfer_progress`, both in the model database. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(model-migration-change-streams)=
## Model migration change streams

Model migration change streams are the notifications that tell the agents of the model and the workers of the controllers that the migration has moved. An agent of the model watches the migrating marker and the migration status. A watcher on the marker tells it that a migration has begun. The status watcher tells it each phase and status message, so that the agent stops its work in `quiesce` and checks the target in `validation`. The migration master of the source watches the phases of its export. The phase history is what the change log records; the status messages are kept out of it, because they change often and would drown the watchers.

(model-migration-operations)=
## Model migration operations

Model migration operations are described on the {ref}`model <model-migrating>` page: a client asks the source controller for a migration; the page states the required access, the rules of every phase, and the errors of the prechecks. The {ref}`external controller <external-controller>` page states how the records of the target controller come to be. The {ref}`lease <lease-and-model-migration>` page states which leases travel.
