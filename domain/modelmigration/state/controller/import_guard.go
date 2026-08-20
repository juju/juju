// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// importPhaseQuery is the single importing-phase assertion used by both
// the txn guard and the companion-table write methods.
const importPhaseQuery = `
SELECT mmipt.type AS &importPhaseRow.phase_type
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $importClaimKey.model_uuid
AND    mmi.uuid = $importClaimKey.claim_uuid
`

// NewImportTxnRunnerFactory returns a transaction runner factory that fences
// every SQLair transaction to one exact target-side import attempt. The claim
// assertion and the caller's work execute in the same transaction.
//
// The phase-assertion statement is prepared eagerly here rather than per
// transaction. The query is a static constant, so a prepare failure is
// permanent (replayed from every factory call) and signals a schema mismatch
// that would defeat every guarded transaction; surfacing it on the first
// factory use is deliberate rather than a delay.
func NewImportTxnRunnerFactory(
	factory coredatabase.TxnRunnerFactory, modelUUID, claimUUID string,
) coredatabase.TxnRunnerFactory {
	stmt, prepareErr := sqlair.Prepare(
		importPhaseQuery, importPhaseRow{}, importClaimKey{},
	)
	return func(ctx context.Context) (coredatabase.TxnRunner, error) {
		if prepareErr != nil {
			return nil, errors.Capture(prepareErr)
		}

		runner, err := factory(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return &importTxnRunner{
			TxnRunner: runner,
			stmt:      stmt,
			modelUUID: modelUUID,
			claimUUID: claimUUID,
		}, nil
	}
}

// importTxnRunner fences SQLair transactions to an import claim. StdTxn is
// promoted from the embedded runner and is not fenced, so callers must use Txn.
// This limitation lasts until the domain runner interface drops StdTxn.
type importTxnRunner struct {
	// TxnRunner supplies Dying and, temporarily, StdTxn. The promoted StdTxn
	// method bypasses the import fence and must not be used with this runner.
	coredatabase.TxnRunner
	stmt      *sqlair.Statement
	modelUUID string
	claimUUID string
}

// Txn asserts that this exact import attempt is still importing before
// executing fn in the same transaction.
func (r *importTxnRunner) Txn(
	ctx context.Context, fn func(context.Context, *sqlair.TX) error,
) error {
	return r.TxnRunner.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := assertImportingClaim(
			ctx, tx, r.stmt, r.modelUUID, r.claimUUID,
		); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

// assertImportingClaim returns nil only while the exact
// model_migration_import claim is in the importing phase.
func assertImportingClaim(
	ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, modelUUID, claimUUID string,
) error {
	arg := importClaimKey{ModelUUID: modelUUID, ClaimUUID: claimUUID}
	var row importPhaseRow
	err := tx.Query(ctx, stmt, arg).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"model %q import claim %q: %w",
			modelUUID, claimUUID, modelmigrationerrors.ErrImportNotFound,
		)
	} else if err != nil {
		return errors.Errorf(
			"checking import claim %q for model %q: %w",
			claimUUID, modelUUID, err,
		)
	}
	if modelmigration.ImportPhase(row.PhaseType) != modelmigration.ImportPhaseImporting {
		return errors.Errorf(
			"model %q import claim %q is %q: %w",
			modelUUID, claimUUID, row.PhaseType,
			modelmigrationerrors.ErrImportNotImporting,
		)
	}
	return nil
}
