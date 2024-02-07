// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/objectstore"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/database"
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
		return objectstore.Metadata{}, errors.Trace(err)
	}

	query := `
SELECT p.path, p.metadata_uuid, m.size, m.hash
FROM object_store_metadata_path p
LEFT JOIN object_store_metadata m ON p.metadata_uuid = m.uuid
WHERE path = ?`

	var metadata objectstore.Metadata
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, query, path)
		return row.Scan(&metadata.Path, &metadata.UUID, &metadata.Size, &metadata.Hash)
	})
	if err != nil {
		return objectstore.Metadata{}, errors.Annotatef(domain.CoerceError(err), "retrieving metadata %s", path)
	}
	return metadata, nil
}

// ListMetadata returns the persistence metadata.
func (s *State) ListMetadata(ctx context.Context) ([]objectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	query := `
SELECT p.path, p.metadata_uuid, m.size, m.hash
FROM object_store_metadata_path p
LEFT JOIN object_store_metadata m ON p.metadata_uuid = m.uuid`

	var metadata []objectstore.Metadata
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("retrieving metadata: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var m objectstore.Metadata
			if err := rows.Scan(&m.Path, &m.UUID, &m.Size, &m.Hash); err != nil {
				return fmt.Errorf("retrieving metadata: %w", err)
			}
			metadata = append(metadata, m)
		}

		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("retrieving metadata: %w", err)
	}
	return metadata, nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *State) PutMetadata(ctx context.Context, metadata objectstore.Metadata) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
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
			return errors.Annotatef(err, "inserting metadata")
		}

		if rows, err := result.RowsAffected(); err != nil {
			return errors.Annotatef(err, "inserting metadata")
		} else if rows != 1 {
			// If the rows affected is 0, then the metadata already exists.
			// We need to get the uuid for the metadata, so that we can insert
			// the path based on that uuid.
			row := tx.QueryRowContext(ctx, metadataLookupQuery, metadata.Hash, metadata.Size)
			if err := row.Scan(&metadata.UUID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return objectstoreerrors.ErrHashAndSizeAlreadyExists
				}
				return errors.Annotatef(err, "inserting metadata")
			}
		}

		result, err = tx.ExecContext(ctx, pathQuery, metadata.Path, metadata.UUID)
		if err != nil {
			return errors.Annotatef(err, "inserting metadata path")
		}
		if rows, err := result.RowsAffected(); err != nil {
			return errors.Annotatef(err, "inserting metadata path")
		} else if rows != 1 {
			return fmt.Errorf("metadata path not inserted")
		}
		return nil
	})
	if err != nil {
		if database.IsErrConstraintPrimaryKey(err) {
			return objectstoreerrors.ErrHashAlreadyExists
		}
		return errors.Annotatef(domain.CoerceError(err), "adding path %s", metadata.Path)
	}
	return nil
}

// RemoveMetadata removes the specified key for the persistence path.
func (s *State) RemoveMetadata(ctx context.Context, path string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
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
		return errors.Annotatef(domain.CoerceError(err), "removing path %s", path)
	}
	return nil
}

// ListMetadata returns the persistence metadata.
func (s *State) ListMetadata(ctx context.Context) ([]objectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `
SELECT p.path, p.metadata_uuid, m.size, m.hash
FROM object_store_metadata_path p
LEFT JOIN object_store_metadata m ON p.metadata_uuid = m.uuid
`

	var metadata []objectstore.Metadata
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m objectstore.Metadata
			if err := rows.Scan(&m.Path, &m.UUID, &m.Size, &m.Hash); err != nil {
				return err
			}
			metadata = append(metadata, m)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Annotatef(domain.CoerceError(err), "listing metadata")
	}
	return metadata, nil
}

// InitialWatchStatement returns the initial watch statement for the
// persistence path.
func (s *State) InitialWatchStatement() (string, string) {
	return "object_store_metadata_path", "SELECT path FROM object_store_metadata_path"
}
