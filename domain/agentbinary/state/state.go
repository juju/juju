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
// It always overwrites the metadata for the given version and arch if it already exists.
// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
func (s *State) Add(ctx context.Context, metadata agentbinary.Metadata) error {
	// Prepare the statements we'll need
	archStmt, err := s.Prepare(`
		SELECT id AS &architectureRecord.id
		FROM architecture
		WHERE name = $architectureRecord.name
	`, architectureRecord{})
	if err != nil {
		return errors.Capture(err)
	}

	existsStmt, err := s.Prepare(`
		SELECT COUNT(*) > 0 AS &objectStoreMeta.exists
		FROM object_store_metadata 
		WHERE uuid = $objectStoreMeta.uuid
	`, objectStoreMeta{})
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
		ON CONFLICT (version, architecture_id) 
		DO UPDATE SET object_store_uuid = $agentBinaryRecord.object_store_uuid
	`, agentBinaryRecord{})
	if err != nil {
		return errors.Capture(err)
	}

	return s.RunAtomic(ctx, func(txCtx domain.AtomicContext) error {
		return domain.Run(txCtx, func(ctx context.Context, tx *sqlair.TX) error {
			// First, check if the architecture exists and get its ID
			archRecord := architectureRecord{Name: metadata.Arch}
			err := tx.Query(ctx, archStmt, archRecord).Get(&archRecord)
			if err == sqlair.ErrNoRows {
				return errors.Errorf("architecture %q", metadata.Arch).Add(coreerrors.NotSupported)
			} else if err != nil {
				return errors.Capture(err)
			}

			// Then check if the object store UUID exists
			objStoreMeta := objectStoreMeta{UUID: string(metadata.ObjectStoreUUID)}
			err = tx.Query(ctx, existsStmt, objStoreMeta).Get(&objStoreMeta)
			if err != nil {
				return errors.Capture(err)
			}
			if !objStoreMeta.Exists {
				return errors.Errorf("object store UUID %q", metadata.ObjectStoreUUID).Add(coreerrors.NotFound)
			}

			// Insert or update the agent binary metadata
			input := agentBinaryRecord{
				Version:         metadata.Version,
				ArchitectureID:  archRecord.ID,
				ObjectStoreUUID: string(metadata.ObjectStoreUUID),
			}
			err = tx.Query(ctx, insertStmt, input).Run()
			if err != nil {
				return errors.Errorf("adding agent binary metadata for version %q and architecture %q: %w",
					metadata.Version, metadata.Arch, err)
			}

			return nil
		})
	})
}
