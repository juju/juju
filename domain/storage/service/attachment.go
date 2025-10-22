// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// GetStorageAttachmentUUIDForStorageIDAndUnit returns the
// [domainstorageprovisioning.StorageAttachmentUUID] associated with the given
// storage instance id and unit name.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when either of the supplied uuids did not pass
// validation.
// - [github.com/juju/juju/domain/storage/errors.StorageNotFound] if the storage
// instance for the supplied uuid no longer exists.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the unit
// no longer exists for the supplied uuid.
func (s *Service) GetStorageAttachmentUUIDForStorageInstanceAndUnit(
	ctx context.Context,
	uuid domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) (domainstorageprovisioning.StorageAttachmentUUID, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if uuid.Validate() != nil {
		return "", errors.New("invalid storage instance uuid").Add(
			coreerrors.NotValid,
		)
	}
	if unitUUID.Validate() != nil {
		return "", errors.New("invalid unit uuid").Add(
			coreerrors.NotValid,
		)
	}

	return s.st.GetStorageAttachmentUUIDForStorageInstanceAndUnit(
		ctx, uuid, unitUUID,
	)
}
