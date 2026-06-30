// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/export/types/latest"
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
	if err := i.state.Import(ctx, payload); err != nil {
		return errors.Errorf("importing model-DB payload: %w", err)
	}
	return nil
}
