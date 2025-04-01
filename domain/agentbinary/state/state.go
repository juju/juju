// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	jujudb "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with agent binaries stored in
// the database.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Add adds a new agent binary's metadata to the database.
// [agentbinaryerrors.AlreadyExists] when the provided agent binary already
// exists.
// [agentbinaryerrors.ObjectNotFound] when no object exists that matches
// this agent binary.
// [coreerrors.NotSupported] if the architecture is not supported by the
// state layer.
func (s *State) Add(ctx context.Context, metadata agentbinary.Metadata) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	archVal := architectureRecord{Name: metadata.Arch}
	agentBinary := agentBinaryRecord{
		Version:         metadata.Version,
		ObjectStoreUUID: metadata.ObjectStoreUUID.String(),
	}

	archStmt, err := s.Prepare(`
SELECT &architectureRecord.*
FROM architecture
WHERE name = $architectureRecord.name
`, archVal)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO agent_binary_store (*) VALUES ($agentBinaryRecord.*)
`, agentBinary)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the object exists in the database for RI.
		exists, err := s.checkObjectExists(ctx, tx, metadata.ObjectStoreUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"object with id %q does not exist in store",
				metadata.ObjectStoreUUID,
			).Add(agentbinaryerrors.ObjectNotFound)
		}

		// Check if the architecture exists and get its ID
		err = tx.Query(ctx, archStmt, archVal).Get(&archVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is not supported",
				metadata.Arch,
			).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Capture(err)
		}

		agentBinary.ArchitectureID = archVal.ID

		err = tx.Query(ctx, insertStmt, agentBinary).Run()
		if jujudb.IsErrConstraintPrimaryKey(err) {
			// There must be an agent version for this version and arch already.
			// We do not want to overwrite this value as it could result in a
			// security risk.
			return errors.Errorf(
				"agent binary of %q already exists", agentBinary.Version,
			).Add(agentbinaryerrors.AlreadyExists)
		} else if err != nil {
			return errors.Capture(err)
		}

		return nil
	})

	if err != nil {
		return errors.Errorf(
			"adding agent binary for version %q and arch %q to state: %w",
			metadata.Version, metadata.Arch, err,
		)
	}
	return nil
}

// checkObjectExists checks if an object exists for the given UUID. True and
// false will be returned with no error indicating if the object exists or not.
func (s *State) checkObjectExists(
	ctx context.Context,
	tx *sqlair.TX,
	objectUUID objectstore.UUID,
) (bool, error) {
	dbVal := objectStoreUUID{UUID: objectUUID.String()}
	objectExistsStmt, err := s.Prepare(`
SELECT &objectStoreUUID.*
FROM object_store_metadata
WHERE uuid = $objectStoreUUID.uuid
`,
		dbVal,
	)
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
