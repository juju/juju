// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	agentbinarystate "github.com/juju/juju/domain/agentbinary/state/internal"
	"github.com/juju/juju/internal/errors"
)

// ModelState represents a type for interacting with the underlying state.
// It works with both controller and model databases.
type ModelState struct {
	*domain.StateBase
}

// NewModelState returns a new ModelState for interacting with agent binaries stored in
// the database.
func NewModelState(factory database.TxnRunnerFactory) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// CheckAgentBinarySHA256Exists checks that the given sha256 sum exists as an
// agent binary in the object store. This sha256 sum could exist as an object in
// the object store but unless the association has been made this will always
// return false.
func (s *ModelState) CheckAgentBinarySHA256Exists(ctx context.Context, sha256Sum string) (bool, error) {
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

// GetObjectUUID returns the object store UUID for the given file path.
// The following errors can be returned:
// - [agentbinaryerrors.ObjectNotFound] when no object exists that matches this path.
func (s *ModelState) GetObjectUUID(
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
// [github.com/juju/juju/core/errors.NotSupported] if the architecture is not
// supported by the state layer.
// [agentbinaryerrors.AgentBinaryImmutable] if an existing agent binary
// already exists with the same version and architecture but a different
// SHA.
func (s *ModelState) RegisterAgentBinary(ctx context.Context, arg agentbinary.RegisterAgentBinaryArg) error {
	return agentbinarystate.RegisterAgentBinary(ctx, s.StateBase, arg, "model")
}

// ListAgentBinaries lists all agent binaries in the state.
// It returns a slice of agent binary metadata.
// An empty slice is returned if no agent binaries are found.
func (s *ModelState) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT &metadataRecord.*
FROM   v_agent_binary_store`, metadataRecord{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var records metadataRecords
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&records)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No agent binaries found.
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("listing agent binary metadata: %w", err)
	}
	return records.toMetadata(), nil
}

// GetAgentBinarySHA256 retrieves the SHA256 value for the specified agent binary version.
// It returns false and an empty string if no matching record exists.
func (s *ModelState) GetAgentBinarySHA256(ctx context.Context, version coreagentbinary.Version, stream agentbinary.Stream) (string, bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	record := metadataRecord{
		Version: version.Number.String(),
		Arch:    version.Arch,
	}

	stmt, err := s.Prepare(`
SELECT &metadataRecord.*
FROM   v_agent_binary_store
WHERE  version = $metadataRecord.version
AND    architecture_name = $metadataRecord.architecture_name`, record)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	exists := false
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, record).Get(&record)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf(
				"checking database to see if agent binary for version %q and architecture %q exists: %w",
				version.Number.String(), version.Arch, err,
			)
		}
		exists = true
		return nil
	})

	if err != nil {
		return "", false, errors.Capture(err)
	}

	return record.SHA256, exists, nil
}

// GetAgentStream returns the stream used by the current model.
// The following errors may be returned:
// - [agentbinaryerrors.StreamNotFound] when the agent binary stream used by the current model is not found.
func (s *ModelState) GetAgentStream(ctx context.Context) (agentbinary.Stream, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	stream := modelAgentStream{}
	stmt, err := s.Prepare(`
SELECT &modelAgentStream.*
FROM   agent_version
`, stream)
	if err != nil {
		return -1, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).Get(&stream)
		if errors.Is(err, sqlair.ErrNoRows) {
			return agentbinaryerrors.StreamNotFound
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	if err != nil {
		return -1, err
	}

	return agentbinary.Stream(stream.StreamID), errors.Capture(err)
}
