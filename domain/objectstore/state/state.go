// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/objectstore"
)

// State implements the domain objectstore state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State instance.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetMetadata returns the persistence metadata for the specified path.
func (s *State) GetMetadata(ctx context.Context, path string) (objectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return objectstore.Metadata{}, err
	}

	query := `
SELECT path, metadata_uuid,  object_store_metadata.size, object_store_metadata.hash
FROM object_store_metadata_path
LEFT JOIN object_store_metadata ON object_store_metadata_path.metadata_uuid = object_store_metadata.uuid
WHERE path = ?`

	var metadata objectstore.Metadata
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, query, path)
		return row.Scan(&metadata.Path, &metadata.UUID, &metadata.Size, &metadata.Hash)
	})
	if err != nil {
		return objectstore.Metadata{}, fmt.Errorf("retrieving metadata %s: %w", path, err)
	}
	return metadata, nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *State) PutMetadata(ctx context.Context, metadata objectstore.Metadata) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	matadataQuery := `
INSERT INTO object_store_metadata (uuid, hash_type_id, hash, size)
VALUES (?, ?, ?, ?) ON CONFLICT (hash) DO NOTHING`
	pathQuery := `
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES (?, ?)`
	metadataLookupQuery := `
SELECT uuid FROM object_store_metadata WHERE hash = ? AND size = ?`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, matadataQuery, metadata.UUID, 1, metadata.Hash, metadata.Size)
		if err != nil {
			return fmt.Errorf("inserting metadata: %w", err)
		}

		if rows, err := result.RowsAffected(); err != nil {
			return fmt.Errorf("inserting metadata: %w", err)
		} else if rows != 1 {
			// If the rows affected is 0, then the metadata already exists.
			// We need to get the uuid for the metadata, so that we can insert
			// the path based on that uuid.
			row := tx.QueryRowContext(ctx, metadataLookupQuery, metadata.Hash, metadata.Size)
			if err := row.Scan(&metadata.UUID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return objectstore.ErrHashAndSizeAlreadyExists
				}
				return fmt.Errorf("inserting metadata: %w", err)
			}
		}

		result, err = tx.ExecContext(ctx, pathQuery, metadata.Path, metadata.UUID)
		if err != nil {
			return fmt.Errorf("inserting metadata path: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil {
			return fmt.Errorf("inserting metadata path: %w", err)
		} else if rows != 1 {
			return fmt.Errorf("metadata path not inserted")
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("adding path %s: %w", metadata.Path, err)
	}
	return nil
}

// RemoveMetadata removes the specified key for the persistence path.
func (s *State) RemoveMetadata(ctx context.Context, path string) error {
	db, err := s.DB()
	if err != nil {
		return err
	}
	metadataUUIDQuery := `SELECT metadata_uuid FROM object_store_metadata_path WHERE path = ?`
	pathQuery := `DELETE FROM object_store_metadata_path WHERE path = ?`
	metadataQuery := `DELETE FROM object_store_metadata WHERE uuid = ? AND NOT EXISTS (SELECT 1 FROM object_store_metadata_path WHERE metadata_uuid = object_store_metadata.uuid)`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// Get the metadata uuid, so we can delete the metadata if there
		// are no more paths associated with it.
		var metadataUUID string
		row := tx.QueryRowContext(ctx, metadataUUIDQuery, path)
		if err := row.Scan(&metadataUUID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, pathQuery, path); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, metadataQuery, metadataUUID); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("removing path %s: %w", path, err)
	}
	return nil
}

// InitialWatchStatement returns the initial watch statement for the
// persistence path.
func (s *State) InitialWatchStatement() string {
	return ""
}
