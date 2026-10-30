---
myst:
  html_meta:
    description: "Juju model migration reference: the phases a model moves through between controllers, what happens in each, who drives them, and the records the move leaves behind: the export and its phases, the import, the redirect, and the model-side markers."
---

(model-migration)=
# Model migration

```{ibnote}
See also: {ref}`Juju | Migrate a model <migrate-a-model>`
```

A **model migration** moves a {ref}`model <model>` from one {ref}`controller <controller>`, the source, to another, the target, through a fixed sequence of phases. The {ref}`external controller <external-controller>` records name the target. The source controller keeps the history of its export attempts and, when the model is gone, a redirect to the target; the target keeps the state of its import; the model's own database keeps the markers of the move.

(model-migration-phases)=
## Model migration phases

```{ggarch}
:file: ../juju.ggarch
:slides: Model migration phase | Migration request | Migration quiesce and import | Migration validation | Migration success and reap
:caption: A migration runs the same phases in the same order every time: the source and the target check the request, the agents quiesce, the target imports the model, the agents validate against the target, and the agents switch to the target before the source removes the model.
:slide-captions: A migration starts in quiesce and moves through import, validation, success, log-transfer, and reap to done. A failure in quiesce, import, or validation moves it to abort and then to abort-done, with the model still on the source controller. A failure when reap removes the model from the source ends in reap-failed, and the model is already live on the target. No other change of phase is allowed. | The client asks the source controller for a migration. The source checks the caller's access and runs its prechecks, the target runs its prechecks, and the source answers with a migration ID. The rest runs without the client. | In `quiesce`, the model's agents stop their work and report that they are ready, and the prechecks run again. In `import`, the source sends the model, its charms, agent binaries, and resources to the target, which holds the model inactive. | In `validation`, the agents log in to the target and report. The target checks the model's machines against the cloud and prepares to activate the model. A failure up to here aborts the migration. | In `success`, the agents report and switch to the target, and the target takes over the cloud resources. The source then copies the logs and removes the model, keeping a redirect record.
:alt: Five slides. The first is a state diagram of ten phases. A row of quiesce, import, validation, success, log-transfer, and reap, each leading to the next. Reap leads to done and to reap-failed. Quiesce, import, and validation each also lead to abort, which leads to abort-done. The other four are sequence diagrams. The first has the client, the source controller, and the target controller: the client requests the migration, the source runs its prechecks, the target runs its prechecks, and the source returns a migration ID. The second has the source, the model agents, and the target: the agents quiesce and report ready, the prechecks run again, and the source sends the model to the target. The third has the source, the agents, the target, and the cloud: the agents log in to the target and report, and the target checks the machines against the cloud and is prepared to activate the model. The fourth has the source, the agents, and the target: the agents report and switch to the target, the target takes over the cloud resources, the source copies the logs, and removes the model.
```

A migration starts in `quiesce`, then moves through `import`, `validation`, `success`, `log-transfer`, and `reap` to `done`. A failure in `quiesce`, `import`, or `validation` moves it to `abort` and then to `abort-done`, with the model still on the source controller. A failure when `reap` removes the model from the source ends in `reap-failed`; the model is already live on the target. No other change of phase is allowed. What happens in each phase:

- **`quiesce`:** The model's {ref}`agents <agent>` stop their work and report that they are ready. The source and the target run their prechecks again. An agent that reports failure, or that does not report within the `migration-agent-wait-time` controller configuration, aborts the migration.
- **`import`:** The source sends the model, its charms, agent binaries, and resources to the target. The target holds the model inactive while the import claim exists; the model becomes usable when the import completes and the claim is released.
- **`validation`:** The agents log in to the target and report. The target checks the model's machines against the cloud and prepares to activate the model. A failure up to here aborts the migration: the source removes the imported copy from the target and the migration ends in `abort-done`, with the model still on the source.
- **`success`:** The agents report and switch to the target. The target takes over the model's cloud resources.
- **`log-transfer`:** The source copies the model's logs to the target, unless the controller forwards logs to [Loki](https://grafana.com/oss/loki/).
- **`reap`:** The source removes the model and keeps a {ref}`redirect <model-migration-database-schema>` with the target's addresses and the model's users, so that agents that were offline learn where the model went. A failure here ends in `reap-failed`; the model is already live on the target.

Each phase writes a status message, which the model's details report as the migration status.

(model-migration-drivers)=
## Who drives a migration

The source controller runs the migration and the target controller receives the model. Three parts drive the phases:

- **The migration master:** A {ref}`worker <controller-workers>` that the source controller runs for the model. It moves the machine from phase to phase, waits for the agents' reports in `quiesce`, `validation`, and `success`, and writes each phase's status message.
- **The `MigrationTarget` facade:** The target controller's part. The master calls it to run the target's prechecks, to import the model, to activate it, and, when a failure aborts the migration, to remove the imported copy.
- **The `MigrationMinion` facade:** The model's part. Each machine and unit {ref}`agent <agent>` watches the migration through it, acts on the phases `quiesce` and `validation`, and reports each phase back, so that the waiting phases wait for every agent to answer.

A client only starts the migration. {ref}`Migrating models <model-migrating>` returns as soon as the migration has an ID. The rest runs without the client; the progress is read from the model's details and the {ref}`logs <log>`.

(model-migration-database-schema)=
## Model migration database schema

```{ggarch}
:file: ../juju.ggarch
:view: Model migration database schema
:caption: An export is one attempt to move a model: read the current phase first, then the history of phases entered, then the per-agent reports for each phase. The offers and the target authorizations are what the move needs. The external controller is another page's record, shown for context.
:alt: Eight boxes of the same size inside a group named Source controller database, each with a filled table mark in the corner. Export is in the middle. Migration phase is above Export (m to 1) and Export phase is to the right of Migration phase, joined to Export (m to 1) and to Migration phase (m to 1). External controller is to the left of Export (m to 1), with Target authorization below it (1 to 1). Status message is to the right of Export (1 to 1), with Exported offer below it (m to 1). Minion report is below Export, joined to Export (m to 1) and to Migration phase (m to 1). External controller has an outlined arrow mark for the record that another page describes.
```

The model migration database schema is the set of records that the two controllers and the model's own database keep about a migration. The records, with the rules that apply to each, are:

- **Export:** One attempt to send the model to a target, on the source controller: the target, the phase the attempt has reached, when it started, and when it last changed. A retry is a new export.
  - *Rules:*
    - A model has at most one export that has not ended (the phases `reap-failed`, `done`, and `abort-done` end it).
- **Migration phase:** The phases of a migration: `quiesce`, `import`, `validation`, `success`, `log-transfer`, `reap`, `reap-failed`, `done`, `abort`, and `abort-done`. The {ref}`phase machine <model-migration-phases>` shows which phase can follow which.
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
- **Exported offer:** One {ref}`offer <offer>` of the model, captured before the model's database is purged, so that the source can remove the offer's permissions after the model is gone.
  - *Rules:*
    - An export holds each offer once.
- **Target authorization:** The {ref}`user <user>` and the {ref}`credentials <credential>` the export uses to reach the target controller, with whether the user checks were skipped. The material is per migration; the {ref}`external controller <external-controller>` record it names is shared with the cross-model relations.
  - *Rules:*
    - An export has one target authorization.
- **External controller:** The target the model moves to. Another page owns it: see {ref}`external controller <external-controller>`.
- **Redirect:** Where a model went, on the source controller after the model is gone: the migration that moved it, the target controller, its addresses, its certificate, an alias, when the redirect was made, and when the move completed. The record has no link to the model: the model is gone when the redirect is there. It stays until the model moves again: a later move overwrites the record.
  - *Rules:*
    - A model has at most one redirect.
- **Redirect user:** One {ref}`user <user>` of the model at the moment it moved, with the access level the user had. The snapshot is independent of the live permissions, which are removed with the model.
  - *Rules:*
    - A redirect holds each user once.
- **Import:** The incoming model, on the target controller: the migration it came in, its phase (`importing`, `activating`, or `aborting`), and when the phase last changed. The target holds the model inactive for as long as the import claim exists: the model becomes usable when the import completes and the claim is released.
  - *Rules:*
    - The target keeps one import per model.
- **Imported offer:** One {ref}`offer <offer>` that the model consumes, captured so that the target can wire the model's cross-model relations when it activates the model.
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

Model migration change streams are the changes that the {ref}`agents <agent>` of the model and the workers of the {ref}`controllers <controller>` follow, delivered through a {ref}`watcher <watcher>`. A unit agent's watcher on the migration flag fires when the export of the model changes phase, and when an import claim for the model appears or disappears. The status watcher tells it each phase, with the addresses of both controllers, so that the agent stops its work in `quiesce` and checks the target in `validation`. The migration master of the source watches the phases of its export and the reports of the agents, so that each phase waits for every agent to answer. The phase history is what the change log records; the status messages are kept out of it, because they change often and would drown the watchers.

(model-migration-calls)=
## Model migration calls

The calls of a migration are described on the {ref}`model <model-migrating>` page: a client asks the source controller for a migration; the page states the caller, the rules of the request, and the errors of the prechecks. The {ref}`external controller <external-controller>` page states how the records of the target controller come to be. The {ref}`lease <lease-and-model-migration>` page states which leases travel.
