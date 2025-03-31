// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Add adds a new agent binary's metadata to the database.
// It returns coreerrors.AlreadyExists if a record with the given version and arch already exists.
// It returns coreerrors.NotSupported if the architecture is not found in the database.
// It returns coreerrors.NotFound if object store UUID is not found in the database.
func (s *State) Add(ctx context.Context, metadata agentbinary.Metadata) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	architectureRecord := architectureRecord{Name: metadata.Arch}
	objectStoreMeta := objectStoreMeta{UUID: string(metadata.ObjectStoreUUID)}
	agentBinaryRecord := agentBinaryRecord{
		Version:         metadata.Version,
		ArchitectureID:  architectureRecord.ID,
		ObjectStoreUUID: string(metadata.ObjectStoreUUID),
	}

	// Prepare the statements we'll need
	archStmt, err := s.Prepare(`
SELECT id AS &archRecord.id
FROM architecture
WHERE name = $archRecord.name
`, architectureRecord)
	if err != nil {
		return errors.Capture(err)
	}

	objStoreStmt, err := s.Prepare(`
SELECT uuid AS &objStoreMeta.uuid
FROM object_store_metadata 
WHERE uuid = $objStoreMeta.uuid
`, objectStoreMeta)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO agent_binary_store (
	version,
	architecture_id,
	object_store_uuid
) VALUES (
	$agentBinaryRecord.version,
	$agentBinaryRecord.architecture_id,
	$agentBinaryRecord.object_store_uuid
)
`, agentBinaryRecord)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// First, check if the architecture exists and get its ID
		err := tx.Query(ctx, archStmt, architectureRecord).Get(&architectureRecord)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("architecture %q", metadata.Arch).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Capture(err)
		}

		// Then check if the object store UUID exists
		err = tx.Query(ctx, objStoreStmt, objectStoreMeta).Get(&objectStoreMeta)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("object store UUID not found").
				Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, insertStmt, agentBinaryRecord).Run()
		if err != nil {
			// Check if this is a duplicate key error
			if errors.Is() {
				return errors.New("agent binary metadata already exists").
					Add(coreerrors.AlreadyExists)
			}
			return errors.Errorf("adding agent binary metadata for version %q and architecture %q: %w",
				metadata.Version, metadata.Arch, err)
		}

		return nil
	})
}
