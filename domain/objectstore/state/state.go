// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
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
func (s *State) GetMetadata(ctx context.Context, path string) (coreobjectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Trace(err)
	}

	metadata := dbMetadata{Path: path}

	stmt, err := s.Prepare(`
SELECT (p.path, m.uuid, m.size, m.hash) AS (&dbMetadata.*)
FROM object_store_metadata_path p
LEFT JOIN object_store_metadata m ON p.metadata_uuid = m.uuid
WHERE path = $dbMetadata.path`, metadata)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Annotate(err, "preparing select metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, metadata).Get(&metadata)
	})
	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return coreobjectstore.Metadata{}, objectstoreerrors.ErrNotFound
		}
		return coreobjectstore.Metadata{}, errors.Annotatef(domain.CoerceError(err), "retrieving metadata %s", path)
	}
	return metadata.ToCoreObjectStoreMetadata(), nil
}

// ListMetadata returns the persistence metadata.
func (s *State) ListMetadata(ctx context.Context) ([]coreobjectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	stmt, err := s.Prepare(`
SELECT (p.path, m.uuid, m.size, m.hash) AS (&dbMetadata.*)
FROM object_store_metadata_path p
LEFT JOIN object_store_metadata m ON p.metadata_uuid = m.uuid`, dbMetadata{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select metadata statement")
	}

	var metadata []dbMetadata
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&metadata)
		if err != nil {
			return errors.Annotate(domain.CoerceError(err), "retrieving metadata")
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return transform.Slice(metadata, (dbMetadata).ToCoreObjectStoreMetadata), nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *State) PutMetadata(ctx context.Context, metadata coreobjectstore.Metadata) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		return err
	}

	dbMetadata := dbMetadata{
		UUID:       uuid.String(),
		Hash:       metadata.Hash,
		HashTypeID: 1,
		Size:       metadata.Size,
	}

	dbMetadataPath := dbMetadataPath{
		UUID: uuid.String(),
		Path: metadata.Path,
	}

	metadataStmt, err := s.Prepare(`
INSERT INTO object_store_metadata (uuid, hash_type_id, hash, size)
VALUES ($dbMetadata.*) ON CONFLICT (hash) DO NOTHING`, dbMetadata)
	if err != nil {
		return errors.Annotate(err, "preparing insert metadata statement")
	}

	pathStmt, err := s.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, dbMetadataPath)
	if err != nil {
		return errors.Annotate(err, "preparing insert metadata path statement")
	}

	metadataLookupStmt, err := s.Prepare(`
SELECT uuid AS &dbMetadataPath.metadata_uuid
FROM   object_store_metadata 
WHERE  hash = $dbMetadata.hash 
AND    size = $dbMetadata.size`, dbMetadata, dbMetadataPath)
	if err != nil {
		return errors.Annotate(err, "preparing select metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, metadataStmt, dbMetadata).Get(&outcome)
		if err != nil {
			return errors.Annotatef(err, "inserting metadata")
		}

		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Annotatef(err, "inserting metadata")
		} else if rows != 1 {
			// If the rows affected is 0, then the metadata already exists.
			// We need to get the uuid for the metadata, so that we can insert
			// the path based on that uuid.
			err := tx.Query(ctx, metadataLookupStmt, dbMetadata).Get(&dbMetadataPath)
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrHashAndSizeAlreadyExists
			}
			return errors.Annotatef(err, "inserting metadata")
		}

		err = tx.Query(ctx, pathStmt, dbMetadataPath).Get(&outcome)
		if err != nil {
			return errors.Annotatef(err, "inserting metadata path")
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
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

	dbMetadataPath := dbMetadataPath{
		Path: path,
	}

	metadataUUIDStmt, err := s.Prepare(`
SELECT &dbMetadataPath.metadata_uuid 
FROM object_store_metadata_path 
WHERE path = $dbMetadataPath.path`, dbMetadataPath)
	if err != nil {
		return errors.Annotate(err, "preparing select metadata statement")
	}
	pathStmt, err := s.Prepare(`
DELETE FROM object_store_metadata_path 
WHERE path = $dbMetadataPath.path`, dbMetadataPath)
	if err != nil {
		return errors.Annotate(err, "preparing delete metadata path statement")
	}

	metadataStmt, err := s.Prepare(`
DELETE FROM object_store_metadata 
WHERE uuid = $dbMetadataPath.metadata_uuid 
AND NOT EXISTS (
  SELECT 1 
  FROM   object_store_metadata_path 
  WHERE  metadata_uuid = object_store_metadata.uuid
)`, dbMetadataPath)
	if err != nil {
		return errors.Annotate(err, "preparing delete metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get the metadata uuid, so we can delete the metadata if there
		// are no more paths associated with it.
		err := tx.Query(ctx, metadataUUIDStmt, dbMetadataPath).Get(&dbMetadataPath)
		if errors.Is(err, sqlair.ErrNoRows) {
			return objectstoreerrors.ErrNotFound
		} else if err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, pathStmt, dbMetadataPath).Run(); err != nil {
			return errors.Trace(err)
		}

		if err := tx.Query(ctx, metadataStmt, dbMetadataPath).Run(); err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	if err != nil {
		return errors.Annotatef(domain.CoerceError(err), "removing path %s", path)
	}
	return nil
}

// InitialWatchStatement returns the initial watch statement for the
// persistence path.
func (s *State) InitialWatchStatement() (string, string) {
	return "object_store_metadata_path", "SELECT path FROM object_store_metadata_path"
}
