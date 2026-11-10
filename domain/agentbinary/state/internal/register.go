// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"

	"github.com/canonical/sqlair"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/internal/errors"
)

// RegisterAgentBinary registers an immutable agent binary mapping in either
// the controller or model database.
func RegisterAgentBinary(ctx context.Context, state *domain.StateBase, arg agentbinary.RegisterAgentBinaryArg, scope string) error {
	db, err := state.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	archVal := architectureRecord{Name: arg.Arch}
	archStmt, err := state.Prepare(`
SELECT &architectureRecord.*
FROM   architecture
WHERE  name = $architectureRecord.name
`, archVal)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := state.Prepare(`
INSERT INTO agent_binary_store (*)
VALUES ($agentBinaryRecord.*)
ON CONFLICT (version, architecture_id) DO NOTHING
`, agentBinaryRecord{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := checkObjectExists(ctx, state, tx, arg.ObjectStoreUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"object with id %q does not exist in store",
				arg.ObjectStoreUUID,
			).Add(agentbinaryerrors.ObjectNotFound)
		}

		err = tx.Query(ctx, archStmt, archVal).Get(&archVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is not supported",
				arg.Arch,
			).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Capture(err)
		}

		agentBinary := agentBinaryRecord{
			Version:         arg.Version,
			ObjectStoreUUID: arg.ObjectStoreUUID.String(),
			ArchitectureID:  archVal.ID,
		}
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, insertStmt, agentBinary).Get(&outcome); err != nil {
			return errors.Capture(err)
		}
		rows, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if rows == 0 {
			// Compare object UUIDs without overwriting the immutable binary.
			existingAgentBinary, err := getAgentBinary(
				ctx, state, tx, agentBinary.Version, agentBinary.ArchitectureID,
			)
			if err != nil {
				return errors.Capture(err)
			}
			if existingAgentBinary.ObjectStoreUUID != agentBinary.ObjectStoreUUID {
				return errors.Errorf(
					"agent binary for version %q and arch %v already exists with a different SHA",
					agentBinary.Version, agentBinary.ArchitectureID,
				).Add(agentbinaryerrors.AgentBinaryImmutable)
			}
			return errors.New(
				"agent binary already exists",
			).Add(agentbinaryerrors.AlreadyExists)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf(
			"adding agent binary for version %q and arch %q to %s state: %w",
			arg.Version, arg.Arch, scope, err,
		)
	}
	return nil
}

func getAgentBinary(
	ctx context.Context,
	state *domain.StateBase,
	tx *sqlair.TX,
	version string,
	architectureID int,
) (agentBinaryRecord, error) {
	agentBinaryVal := agentBinaryRecord{
		Version:        version,
		ArchitectureID: architectureID,
	}
	getStmt, err := state.Prepare(`
SELECT &agentBinaryRecord.*
FROM   agent_binary_store
WHERE  version = $agentBinaryRecord.version
AND    architecture_id = $agentBinaryRecord.architecture_id
`, agentBinaryVal)
	if err != nil {
		return agentBinaryVal, errors.Capture(err)
	}
	err = tx.Query(ctx, getStmt, agentBinaryVal).Get(&agentBinaryVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return agentBinaryVal, errors.Errorf(
			"agent binary for version %q and arch %d not found",
			agentBinaryVal.Version, agentBinaryVal.ArchitectureID,
		).Add(coreerrors.NotFound)
	}
	if err != nil {
		return agentBinaryVal, errors.Errorf(
			"getting agent binary for version %q and arch %d: %w",
			agentBinaryVal.Version, agentBinaryVal.ArchitectureID, err,
		)
	}
	return agentBinaryVal, nil
}

func checkObjectExists(
	ctx context.Context,
	state *domain.StateBase,
	tx *sqlair.TX,
	objectUUID objectstore.UUID,
) (bool, error) {
	dbVal := objectStoreUUID{UUID: objectUUID.String()}
	objectExistsStmt, err := state.Prepare(`
SELECT &objectStoreUUID.*
FROM   object_store_metadata
WHERE  uuid = $objectStoreUUID.uuid
`, dbVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, objectExistsStmt, dbVal).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Errorf(
			"checking if object for store uuid %q exists: %w",
			objectUUID, err,
		)
	}
	return true, nil
}

type architectureRecord struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type objectStoreUUID struct {
	UUID string `db:"uuid"`
}

type agentBinaryRecord struct {
	Version         string `db:"version"`
	ArchitectureID  int    `db:"architecture_id"`
	ObjectStoreUUID string `db:"object_store_uuid"`
}
