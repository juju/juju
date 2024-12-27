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
SELECT &dbMetadata.*
FROM v_object_store_metadata
WHERE path = $dbMetadata.path`, metadata)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Annotate(err, "preparing select metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, metadata).Get(&metadata)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrNotFound
			}
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Annotatef(err, "retrieving metadata %s", path)
	}
	return decodeDbMetadata(metadata), nil
}

// GetMetadataBySHA256Prefix returns the persistence metadata for the object
// with SHA256 starting with the provided prefix.
func (s *State) GetMetadataBySHA256Prefix(ctx context.Context, sha256 string) (coreobjectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Trace(err)
	}

	sha256Prefix := sha256Prefix{SHA256Prefix: sha256}
	var metadata dbMetadata

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata
WHERE sha_256 LIKE $sha256Prefix.sha_256_prefix || '%'`, metadata, sha256Prefix)
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Annotate(err, "preparing select metadata statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sha256Prefix).Get(&metadata)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return objectstoreerrors.ErrNotFound
			}
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return coreobjectstore.Metadata{}, errors.Annotatef(err, "retrieving metadata with sha256 %s", sha256)
	}

	return decodeDbMetadata(metadata), nil
}

// ListMetadata returns the persistence metadata.
func (s *State) ListMetadata(ctx context.Context) ([]coreobjectstore.Metadata, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	stmt, err := s.Prepare(`
SELECT &dbMetadata.*
FROM v_object_store_metadata`, dbMetadata{})
	if err != nil {
		return nil, errors.Annotate(err, "preparing select metadata statement")
	}

	var metadata []dbMetadata
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&metadata)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotate(err, "retrieving metadata")
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return transform.Slice(metadata, decodeDbMetadata), nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *State) PutMetadata(ctx context.Context, metadata coreobjectstore.Metadata) (coreobjectstore.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	uuid, err := coreobjectstore.NewUUID()
	if err != nil {
		return "", err
	}

	dbMetadata := dbMetadata{
		UUID:   uuid.String(),
		SHA256: metadata.SHA256,
		SHA384: metadata.SHA384,
		Size:   metadata.Size,
	}

	dbMetadataPath := dbMetadataPath{
		UUID: uuid.String(),
		Path: metadata.Path,
	}

	metadataStmt, err := s.Prepare(`
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES ($dbMetadata.*) 
ON CONFLICT (sha_256) DO NOTHING
ON CONFLICT (sha_384) DO NOTHING`, dbMetadata)
	if err != nil {
		return "", errors.Annotate(err, "preparing insert metadata statement")
	}

	pathStmt, err := s.Prepare(`
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES ($dbMetadataPath.*)`, dbMetadataPath)
	if err != nil {
		return "", errors.Annotate(err, "preparing insert metadata path statement")
	}

	metadataLookupStmt, err := s.Prepare(`
SELECT uuid AS &dbMetadataPath.metadata_uuid
FROM   object_store_metadata 
WHERE  (
	sha_384 = $dbMetadata.sha_384 OR
	sha_256 = $dbMetadata.sha_256
)
AND    size = $dbMetadata.size`, dbMetadata, dbMetadataPath)
	if err != nil {
		return "", errors.Annotate(err, "preparing select metadata statement")
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
			} else if err != nil {
				return errors.Annotatef(err, "inserting metadata")
			}
			// At this point we need to update the uuid that we'll
			// return back to be the one that was already in the db.
			uuid, err = coreobjectstore.ParseUUID(dbMetadataPath.UUID)
			if err != nil {
				return errors.Annotatef(err, "parsing present uuid in metadata")
			}
		}

		err = tx.Query(ctx, pathStmt, dbMetadataPath).Get(&outcome)
		if database.IsErrConstraintPrimaryKey(err) {
			return objectstoreerrors.ErrPathAlreadyExistsDifferentHash
		} else if err != nil {
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
		return "", errors.Annotatef(err, "adding path %s", metadata.Path)
	}
	return uuid, nil
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
		return errors.Annotatef(err, "removing path %s", path)
	}
	return nil
}

// InitialWatchStatement returns the initial watch statement for the
// persistence path.
func (s *State) InitialWatchStatement() (string, string) {
	return "object_store_metadata_path", "SELECT path FROM object_store_metadata_path"
}
