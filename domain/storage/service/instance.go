// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// GetStorageInstanceInfo returns the basic information about a StorageInstance
// in the model.
func (s *Service) GetStorageInstanceInfo(
	ctx context.Context, uuid domainstorage.StorageInstanceUUID,
) (domainstorage.StorageInstanceInfo, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return domainstorage.StorageInstanceInfo{}, errors.New("not yet implemented")
}

// GetStorageInstanceUUIDForID returns the StorageInstanceUUID for the given
// storage ID.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound] if no
// storage instance exists for the provided storage id.
func (s *Service) GetStorageInstanceUUIDForID(
	ctx context.Context, storageID string,
) (domainstorage.StorageInstanceUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// We don't have any validation that we run over storage id's at the moment.
	return s.st.GetStorageInstanceUUIDByID(ctx, storageID)
}
