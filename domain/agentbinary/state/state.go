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

// GetObjectUUID returns the object store UUID for the given file path.
// The following errors can be returned:
// - [agentbinaryerrors.ObjectNotFound] when no object exists that matches this path.
func (s *State) GetObjectUUID(
	ctx context.Context,
	path string,
) (objectstore.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	type objectStorePath struct {
		Path         string           `db:"path"`
		MetadataUUID objectstore.UUID `db:"metadata_uuid"`
	}
	objectStore := objectStorePath{Path: path}

	stmt, err := s.Prepare(`
SELECT metadata_uuid AS &objectStorePath.metadata_uuid
FROM   object_store_metadata_path
WHERE  path = $objectStorePath.path`, objectStore)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, objectStore).Get(&objectStore)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"object with path %q not found in store",
				objectStore.Path,
			).Add(agentbinaryerrors.ObjectNotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", errors.Errorf(
			"getting object with path %q: %w",
			objectStore.Path, err,
		)
	}
	return objectStore.MetadataUUID, nil
}

// AddAgentBinary adds a new agent binary's metadata to the database.
// [agentbinaryerrors.AlreadyExists] when the provided agent binary already
// exists.
// [agentbinaryerrors.ObjectNotFound] when no object exists that matches
// this agent binary.
// [coreerrors.NotSupported] if the architecture is not supported by the
// state layer.
// [agentbinaryerrors.AgentBinaryImmutable] if an existing agent binary
// already exists with the same version and architecture but a different
// SHA.
func (s *State) AddAgentBinary(ctx context.Context, arg agentbinary.AddAgentBinaryArg) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	archVal := architectureRecord{Name: arg.Arch}
	agentBinary := agentBinaryRecord{
		Version:         arg.Version,
		ObjectStoreUUID: arg.ObjectStoreUUID.String(),
	}

	archStmt, err := s.Prepare(`
SELECT &architectureRecord.*
FROM   architecture
WHERE  name = $architectureRecord.name
`, archVal)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO agent_binary_store (*) 
VALUES ($agentBinaryRecord.*)
`, agentBinary)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the object exists in the database for RI.
		exists, err := s.checkObjectExists(ctx, tx, arg.ObjectStoreUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"object with id %q does not exist in store",
				arg.ObjectStoreUUID,
			).Add(agentbinaryerrors.ObjectNotFound)
		}

		// Check if the architecture exists and get its ID
		err = tx.Query(ctx, archStmt, archVal).Get(&archVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is not supported",
				arg.Arch,
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

			// We want to check if the supplied SHA is a different value.
			// If it is, we want to return the agentbinaryerrors.AgentBinaryImmutable error.
			// The agent binary is immutable once it is uploaded.
			existingAgentBinary, err := s.getAgentBinary(
				ctx, tx, agentBinary.Version, agentBinary.ArchitectureID,
			)
			if errors.Is(err, coreerrors.NotFound) {
				// This should never happen.
				return errors.Capture(err)
			}
			if err != nil {
				return errors.Capture(err)
			}
			if existingAgentBinary.ObjectStoreUUID != agentBinary.ObjectStoreUUID {
				return errors.Errorf(
					"agent binary for version %q and arch %q already exists with a different SHA",
					agentBinary.Version, agentBinary.ArchitectureID,
				).Add(agentbinaryerrors.AgentBinaryImmutable)
			}

			return errors.New(
				"agent binary already exists",
			).Add(agentbinaryerrors.AlreadyExists)
		}
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	if err != nil {
		return errors.Errorf(
			"adding agent binary for version %q and arch %q to state: %w",
			arg.Version, arg.Arch, err,
		)
	}
	return nil
}

func (s *State) getAgentBinary(
	ctx context.Context,
	tx *sqlair.TX,
	version string,
	architectureID int,
) (agentBinaryRecord, error) {
	agentBinaryVal := agentBinaryRecord{
		Version:        version,
		ArchitectureID: architectureID,
	}
	getStmt, err := s.Prepare(`
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
			"agent binary for version %q and arch %q not found",
			agentBinaryVal.Version, agentBinaryVal.ArchitectureID,
		).Add(coreerrors.NotFound)
	}
	if err != nil {
		return agentBinaryVal, errors.Errorf(
			"getting agent binary for version %q and arch %q: %w",
			agentBinaryVal.Version, agentBinaryVal.ArchitectureID, err,
		)
	}
	return agentBinaryVal, nil
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
FROM   object_store_metadata
WHERE  uuid = $objectStoreUUID.uuid
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

// ListAgentBinaries lists all agent binaries in the state.
// It returns a slice of agent binary metadata.
// An empty slice is returned if no agent binaries are found.
func (s *State) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	// TODO: Implement this function to list all agent binaries.
	return nil, errors.New("not implemented")
}
