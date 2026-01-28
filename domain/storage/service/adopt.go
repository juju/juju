// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// AdoptFilesystem adopts a filesystem by invoking the provider of the given
// storage pool to identify the filesystem on the given natural entity specified
// by the provider ID (e.g. a filesystem on a volume or a filesystem directly).
// The result of this call is the name of a new storage instance using the given
// storage name.
// The following errors can be expected:
// - [github.com/juju/juju/domain/storage/errors.StoragePoolNotFound] if the
// specified storage pool does not exist.
// - [github.com/juju/juju/domain/storage/errors.PooledStorageEntityNotFound] if
// the pool name is not valid.
func (s *StorageService) AdoptFilesystem(
	ctx context.Context,
	storageName string,
	pool domainstorage.StoragePoolUUID,
	providerID string,
	force bool,
) (corestorage.ID, error) {
	return "", errors.New(
		"AdoptFilesystem is not implemented yet",
	).Add(coreerrors.NotImplemented)
}
