// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	jujudb "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// ControllerState represents a type for interacting with the underlying state.
// It works with both controller and model databases.
type ControllerState struct {
	*domain.StateBase
}

// NewControllerState returns a new ControllerState for interacting with agent binaries stored in
// the database.
func NewControllerState(factory database.TxnRunnerFactory) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
	}
}

// CheckAgentBinarySHA256Exists checks that the given sha256 sum exists as an
// agent binary in the object store. This sha256 sum could exist as an object in
// the object store but unless the association has been made this will always
// return false.
func (s *ControllerState) CheckAgentBinarySHA256Exists(
	ctx context.Context, sha256Sum string,
) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	dbVal := objectStoreSHA256Sum{Sum: sha256Sum}

	stmt, err := s.Prepare(`
SELECT &objectStoreSHA256Sum.*
FROM   v_agent_binary_store
WHERE sha_256 = $objectStoreSHA256Sum.sha_256
`, dbVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	exists := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, dbVal).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf(
				"checking database to see if agent binary for sha256 %q exists: %w",
				sha256Sum, err,
			)
		}
		exists = true
		return nil
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	return exists, nil
}

// GetAllAgentStoreBinariesForStream returns all agent binaries that are
// available in the controller store for a given stream. If no agent binaries
// exist for the stream, an empty slice is returned.
func (s *ControllerState) GetAllAgentStoreBinariesForStream(
	ctx context.Context, stream agentbinary.Stream,
) ([]agentbinary.AgentBinary, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	streamInput := agentStream{StreamID: int(stream)}

	q := `
SELECT &agentStoreBinary.*
FROM (
    SELECT abs.version,
           abs.architecture_id,
           osm.size,
           $agentStream.stream_id AS stream_id,
           osm.sha_256
    FROM   agent_binary_store abs
    JOIN   object_store_metadata osm
)
`

	stmt, err := s.Prepare(q, streamInput, agentStoreBinary{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing get all agent binaries for stream query: %w", err,
		)
	}

	dbVals := []agentStoreBinary{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, streamInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	retVal := make([]agentbinary.AgentBinary, 0, len(dbVals))
	for _, dbVal := range dbVals {
		version, err := semversion.Parse(dbVal.Version)
		if err != nil {
			return nil, errors.Errorf(
				"parsing agent binary version %q: %w",
				dbVal.Version, err,
			)
		}

		retVal = append(retVal, agentbinary.AgentBinary{
			Architecture: agentbinary.Architecture(dbVal.ArchitectureID),
			SHA256:       dbVal.SHA256,
			Stream:       agentbinary.Stream(dbVal.StreamID),
			Version:      version,
		})
	}

	return retVal, nil
}

// GetObjectUUID returns the object store UUID for the given file path.
// The following errors can be returned:
// - [agentbinaryerrors.ObjectNotFound] when no object exists that matches this path.
func (s *ControllerState) GetObjectUUID(
	ctx context.Context,
	path string,
) (objectstore.UUID, error) {
	db, err := s.DB(ctx)
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

// RegisterAgentBinary registers a new agent binary's metadata to the database.
// [agentbinaryerrors.AlreadyExists] when the provided agent binary already
// exists.
// [agentbinaryerrors.ObjectNotFound] when no object exists that matches
// this agent binary.
// [coreerrors.NotSupported] if the architecture is not supported by the
// state layer.
// [agentbinaryerrors.AgentBinaryImmutable] if an existing agent binary
// already exists with the same version and architecture but a different
// SHA.
func (s *ControllerState) RegisterAgentBinary(ctx context.Context, arg agentbinary.RegisterAgentBinaryArg) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	agentBinary := agentBinaryRecord{
		ArchitectureID:  int(arg.Architecture),
		ObjectStoreUUID: arg.ObjectStoreUUID.String(),
		Version:         arg.Version,
	}

	insertStmt, err := s.Prepare(
		`
INSERT INTO agent_binary_store (*)
VALUES ($agentBinaryRecord.*)
`,
		agentBinary,
	)
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
			"adding agent binary for version %q and arch %q to controller state: %w",
			arg.Version, arg.Architecture, err,
		)
	}
	return nil
}

func (s *ControllerState) getAgentBinary(
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
func (s *ControllerState) checkObjectExists(
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
