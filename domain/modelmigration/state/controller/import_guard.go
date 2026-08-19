// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// NewImportTxnRunnerFactory returns a transaction runner factory that fences
// every SQLair transaction to one exact target-side import attempt. The claim
// assertion and the caller's work execute in the same transaction.
func NewImportTxnRunnerFactory(
	factory coredatabase.TxnRunnerFactory, modelUUID, claimUUID string,
) coredatabase.TxnRunnerFactory {
	arg := importClaimKey{ModelUUID: modelUUID, ClaimUUID: claimUUID}
	row := importPhaseRow{}
	stmt, prepareErr := sqlair.Prepare(`
SELECT mmipt.type AS &importPhaseRow.phase_type
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $importClaimKey.model_uuid
AND    mmi.uuid = $importClaimKey.claim_uuid
`, row, arg)

	return func(ctx context.Context) (coredatabase.TxnRunner, error) {
		if prepareErr != nil {
			return nil, errors.Capture(prepareErr)
		}
		runner, err := factory(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return &importTxnRunner{
			runner:    runner,
			stmt:      stmt,
			modelUUID: modelUUID,
			claimUUID: claimUUID,
		}, nil
	}
}

// importTxnRunner guards SQLair transactions and deliberately rejects
// standard-library transactions, which cannot share the SQLair assertion.
type importTxnRunner struct {
	runner    coredatabase.TxnRunner
	stmt      *sqlair.Statement
	modelUUID string
	claimUUID string
}

// Txn asserts that this exact import attempt is still importing before
// executing fn in the same transaction.
func (r *importTxnRunner) Txn(
	ctx context.Context, fn func(context.Context, *sqlair.TX) error,
) error {
	return r.runner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		arg := importClaimKey{ModelUUID: r.modelUUID, ClaimUUID: r.claimUUID}
		var row importPhaseRow
		err := tx.Query(ctx, r.stmt, arg).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"model %q import claim %q: %w",
				r.modelUUID, r.claimUUID, modelmigrationerrors.ErrImportNotFound,
			)
		} else if err != nil {
			return errors.Errorf(
				"checking import claim %q for model %q: %w",
				r.claimUUID, r.modelUUID, err,
			)
		}
		if modelmigration.ImportPhase(row.PhaseType) != modelmigration.ImportPhaseImporting {
			return errors.Errorf(
				"model %q import claim %q is %q: %w",
				r.modelUUID, r.claimUUID, row.PhaseType,
				modelmigrationerrors.ErrImportNotImporting,
			)
		}
		return fn(ctx, tx)
	})
}

// StdTxn always fails closed because its callback cannot share the SQLair
// transaction used by the import assertion.
func (*importTxnRunner) StdTxn(
	context.Context, func(context.Context, *sql.Tx) error,
) error {
	return errors.Errorf("standard transaction for guarded import: %w", coreerrors.NotSupported)
}

// Dying reports the lifetime of the underlying controller database.
func (r *importTxnRunner) Dying() <-chan struct{} {
	return r.runner.Dying()
}
