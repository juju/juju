// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	domainstorage "github.com/juju/juju/domain/storage"
)

// GetStorageInstanceUUIDForID returns the StorageInstanceUUID for the given
// storage ID.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if no storage
// instance exists for the provided storage id.
func (s *Service) GetStorageInstanceUUIDForID(
	ctx context.Context, storageID string,
) (domainstorage.StorageInstanceUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetStorageInstanceUUIDByID(ctx, storageID)
}
