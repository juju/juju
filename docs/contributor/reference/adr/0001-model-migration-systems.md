---
myst:
  html_meta:
    description: "ADR for Juju model migration systems."
---

(adr-0001-model-migration-systems)=
# 0001: Model migration systems

## Status

Draft

## Date

2026-04-30

## Context

Model migration moves a model between controllers while preserving the model's
identity, state, agent coordination, and external-resource relationships. 

The model migration workflow encompasses:
- Coordinating workers across migration phases.
- Transferring model state between controllers.
- Transferring logs and binaries between controllers.
- Verification by agents of migration validity.
- Recording migration metadata.
- Activating successfully migrated models.

Juju 4.0 retains the ability to _import_ models using the same mechanism as 
prior versions, with the data exchange format defined in the 
[description](https://github.com/juju/description/) package. This is referred
to hereafter as "legacy" migration. 

This ability must be maintained in 4.1, which is the intended LTS release for
versions of Juju 4.

Migrations between versions 4.0 or greater will use a "new" mechanism for the 
exchange of model state.

Support requirements for these mechanisms become:
### 3.6 and prior (unchanged)
- legacy import
- legacy export

### 4.0
- legacy import
- new export

### 4.1
- legacy import
- new import
- new export

### 4.2 and beyond
- new import
- new export

We focus here on architectural considerations for supporting both migration
mechanisms across 4.0 and 4.1 versions.

## Decision

TBD.

Record the intended architecture for model migration systems here before the
status changes from Draft to Accepted. At minimum, the accepted decision should
describe:

- Which component owns migration orchestration.
- How model data is exported, transferred, validated, and imported.
- How each domain contributes import and export operations.
- How failures, rollback, and retry are handled.
- Which compatibility guarantees apply across controller versions.
- How agents and workers observe and report migration phases.

## Consequences

TBD.

When the decision is accepted, describe the operational and code-structure
consequences here, including any required package boundaries, migration testing
expectations, and follow-up ADRs.

## Related code

- `core/modelmigration`
- `domain/modelmigration`
- `domain/*/modelmigration`
- `internal/worker/migrationmaster`
- `internal/worker/migrationminion`
