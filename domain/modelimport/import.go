// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	importstate "github.com/juju/juju/domain/modelimport/state/model"
	"github.com/juju/juju/internal/errors"
)

// Importer applies a transformed, target-version model-DB payload to the model
// database. The transformed payload's rows already match the target schema by
// construction, so the importer bulk-inserts every content table directly.
type Importer struct {
	state *importstate.State
}

// NewImporter returns an [Importer] that writes into the model database
// reachable through the given transaction-runner factory.
func NewImporter(modelDB database.TxnRunnerFactory) *Importer {
	return &Importer{
		state: importstate.NewState(modelDB),
	}
}

// Import inserts the transformed model-DB payload into the target model DB. A
// nil payload is a no-op.
func (i *Importer) Import(ctx context.Context, payload *latest.ModelExport) error {
	if payload == nil {
		return nil
	}
	sanitized := sanitizeCharmBlobResidency(*payload)
	if err := i.state.Import(ctx, &sanitized); err != nil {
		return errors.Errorf("importing model-DB payload: %w", err)
	}
	if err := i.applyPostImportFixups(ctx, sanitized); err != nil {
		return errors.Errorf("applying post-import fixups: %w", err)
	}
	return nil
}

// sanitizeCharmBlobResidency returns a copy of payload whose charm rows have
// their blob-residency fields (Available, ArchivePath, ObjectStoreUUID)
// cleared. Those fields describe whether the charm's binary has actually
// landed on this controller, which the source payload cannot answer: only
// the binary-transfer phase that follows Import resolves them, once the
// archive is actually verified here.
//
// This must happen before the payload reaches the generated bulk insert, not
// after: object_store_metadata is deliberately excluded from import (see
// nonContentTables in generate/modelimport/main.go), so an unmodified
// ObjectStoreUUID would reference a row that will never exist on the target,
// failing the deferred foreign-key check when state.Import's own transaction
// commits -- before any later fixup would get a chance to clear it.
func sanitizeCharmBlobResidency(payload latest.ModelExport) latest.ModelExport {
	if len(payload.Charm) == 0 {
		return payload
	}
	notAvailable := false
	charms := make([]v4_1_0.Charm, len(payload.Charm))
	copy(charms, payload.Charm)
	for i := range charms {
		charms[i].Available = &notAvailable
		charms[i].ArchivePath = nil
		charms[i].ObjectStoreUUID = nil
	}
	payload.Charm = charms
	return payload
}

// applyPostImportFixups performs target-side corrections that are Juju
// business logic, not schema-driven content the generated bulk insert in
// state.Import can express: the migrated model's agent password is merged
// into the target's bootstrap model_agent row (that row is target-owned,
// created at bootstrap; only the password travels from the source).
//
// It runs after state.Import's transaction has already committed. Unlike
// sanitizeCharmBlobResidency, this has no foreign-key dependency on an
// excluded table, so it is free to run as a separate, later transaction.
func (i *Importer) applyPostImportFixups(ctx context.Context, payload latest.ModelExport) error {
	if err := ValidatePayload(payload); err != nil {
		return errors.Capture(err)
	}
	if err := i.state.MergeModelAgentPassword(ctx, payload.ModelAgent[0]); err != nil {
		return errors.Errorf("merging model agent password: %w", err)
	}
	return nil
}
