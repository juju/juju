// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// GetStorageAttachmentUUIDForStorageInstanceAndUnit returns the
// [domainstorageprovisioning.StorageAttachmentUUID] associated with the given
// storage instance id and unit name.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when either of the supplied uuids did not pass
// validation.
// - [storageerrors.StorageNotFound] if the storage
// instance for the supplied uuid no longer exists.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the unit
// no longer exists for the supplied uuid.
func (s *Service) GetStorageAttachmentUUIDForStorageInstanceAndUnit(
	ctx context.Context,
	uuid domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
) (domainstorageprovisioning.StorageAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
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

// GetStorageInstanceAttachments returns the set of attachments a storage
// instance has. If the storage instance has no attachments then an empty slice
// is returned.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied uuid did not pass validation.
// - [storageerrors.StorageInstanceNotFound] if the storage instance for the
// supplied uuid does not exist.
func (s *Service) GetStorageInstanceAttachments(
	ctx context.Context,
	uuid domainstorage.StorageInstanceUUID,
) ([]domainstorageprovisioning.StorageAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if uuid.Validate() != nil {
		return nil, errors.New(
			"invalid storage instance uuid",
		).Add(coreerrors.NotValid)
	}

	attachments, err := s.st.GetStorageInstanceAttachments(
		ctx, uuid)
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return nil, errors.Errorf(
			"storage instance %q not found", uuid,
		).Add(storageerrors.StorageInstanceNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"getting attachments of storage instance %q: %w", uuid, err,
		)
	}

	return attachments, nil
}
