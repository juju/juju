// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/resource"
	"github.com/juju/juju/domain/containerimageresourcestore"
	containerimageresourcestoreerrors "github.com/juju/juju/domain/containerimageresourcestore/errors"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory database.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetContainerImageMetadata returns the container image metadata with the given
// UUID. containerimageresourcestoreerrors.ContainerImageMetadataNotFound is
// returned if the UUID is not in the table.
func (s *State) GetContainerImageMetadata(
	ctx context.Context,
	storageKey string,
) (containerimageresourcestore.ContainerImageMetadata, error) {
	db, err := s.DB()
	if err != nil {
		return containerimageresourcestore.ContainerImageMetadata{}, errors.Capture(err)
	}

	sk := containerImageMetadataStorageKey{StorageKey: storageKey}
	m := containerImageMetadata{}
	stmt, err := s.Prepare(`
SELECT &containerImageMetadata.*
FROM   resource_container_image_metadata_store
WHERE  storage_key = $containerImageMetadataStorageKey.storage_key
`, sk, m)
	if err != nil {
		return containerimageresourcestore.ContainerImageMetadata{}, errors.Errorf(
			"preparing select container image resource metadata statement: %w",
			err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, sk).Get(&m)
		if errors.Is(err, sqlair.ErrNoRows) {
			return containerimageresourcestoreerrors.ContainerImageMetadataNotFound
		}
		return err
	})
	if err != nil {
		return containerimageresourcestore.ContainerImageMetadata{}, err
	}

	return containerimageresourcestore.ContainerImageMetadata{
		StorageKey:   m.StorageKey,
		RegistryPath: m.RegistryPath,
		Username:     m.UserName,
		Password:     m.Password,
	}, nil
}

// PutContainerImageMetadata stores container image metadata the database and
// returns its UUID.
func (s *State) PutContainerImageMetadata(
	ctx context.Context,
	storageKey string,
	registryPath, userName, password string,
) (resource.ResourceStorageUUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	m := containerImageMetadata{
		StorageKey:   storageKey,
		RegistryPath: registryPath,
		UserName:     userName,
		Password:     password,
	}

	stmt, err := s.Prepare(`
INSERT INTO resource_container_image_metadata_store (*)
VALUES ($containerImageMetadata.*) 
ON CONFLICT (storage_key) DO UPDATE SET
      registry_path = excluded.registry_path,
      username  = excluded.username,
      password  = excluded.password
WHERE storage_key = excluded.storage_key
                                       

`, m)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err != nil {
			return errors.Errorf("preparing upsert container image metadata statement: %w", err)
		}
		var outcome sqlair.Outcome
		err = tx.Query(ctx, stmt, m).Get(&outcome)
		if err != nil {
			return errors.Errorf("upserting container image metadata: %w", err)
		}

		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("getting rows affected by upsert: %w", err)
		} else if rows != 1 {
			return errors.Errorf(
				"updating existing container image metadata with storage path %s: %d rows affected",
				m.StorageKey, rows,
			)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return resource.ResourceStorageUUID(storageKey), nil
}

// RemoveContainerImageMetadata removes a container image metadata resource from
// storage. containerimageresourcestoreerrors.ContainerImageMetadataNotFound is
// returned if the resource does not exist.
func (s *State) RemoveContainerImageMetadata(ctx context.Context, storageKey string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	sk := containerImageMetadataStorageKey{StorageKey: storageKey}
	stmt, err := s.Prepare(`
DELETE FROM resource_container_image_metadata_store 
WHERE       storage_key = $containerImageMetadataStorageKey.storage_key
`, sk)
	if err != nil {
		return errors.Errorf(
			"preparing remove container image resource metadata statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, stmt, sk).Get(&outcome)
		if err != nil {
			return err
		}
		if rows, err := outcome.Result().RowsAffected(); err != nil {
			return err
		} else if rows == 0 {
			return containerimageresourcestoreerrors.ContainerImageMetadataNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
