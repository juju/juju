// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// GetStorageInstanceInfo returns the basic information about a StorageInstance
// in the model and its attachments onto Unit's.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/storage/errors.StorageInstanceNotFound] when
// the Storage Instance does not exist in the model.
// - [coreerrors.NotValid] when the supplied Storage Instance UUID is not valid.
func (s *Service) GetStorageInstanceInfo(
	ctx context.Context, uuid domainstorage.StorageInstanceUUID,
) (domainstorage.StorageInstanceInfo, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return domainstorage.StorageInstanceInfo{}, errors.New(
			"storage instance uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	internalInfo, err := s.st.GetStorageInstanceInfo(ctx, uuid)
	if err != nil {
		return domainstorage.StorageInstanceInfo{}, err
	}

	retVal := domainstorage.StorageInstanceInfo{
		ID:   internalInfo.StorageID,
		Life: internalInfo.Life,
		Kind: internalInfo.Kind,
		UUID: internalInfo.UUID,
	}

	if internalInfo.Filesystem != nil && internalInfo.Filesystem.Status != nil {
		retVal.FilesystemStatus = &domainstorage.StorageInstanceFilesystemStatus{
			Status:  internalInfo.Filesystem.Status.Status.ToCoreStatus(),
			Message: internalInfo.Filesystem.Status.Message,
			Since:   internalInfo.Filesystem.Status.UpdatedAt,
			UUID:    internalInfo.Filesystem.UUID,
		}
	}
	if internalInfo.Volume != nil {
		retVal.Persistent = internalInfo.Volume.Persistent
	}
	if internalInfo.Volume != nil && internalInfo.Volume.Status != nil {
		retVal.VolumeStatus = &domainstorage.StorageInstanceVolumeStatus{
			Status:  internalInfo.Volume.Status.Status.ToCoreStatus(),
			Message: internalInfo.Volume.Status.Message,
			Since:   internalInfo.Volume.Status.UpdatedAt,
			UUID:    internalInfo.Volume.UUID,
		}
	}
	if internalInfo.UnitOwner != nil {
		retVal.UnitOwner = &domainstorage.StorageInstanceUnitOwner{
			Name: internalInfo.UnitOwner.Name,
			UUID: internalInfo.UnitOwner.UUID,
		}
	}

	for _, internalAttachment := range internalInfo.Attachments {
		attachment := domainstorage.StorageInstanceUnitAttachmentInfo{
			Life:     internalAttachment.Life,
			UnitName: internalAttachment.UnitName,
			UnitUUID: internalAttachment.UnitUUID,
			UUID:     internalAttachment.UUID,
		}

		if internalAttachment.Machine != nil {
			attachment.MachineAttachment = &domainstorage.StorageInstanceMachineAttachment{
				MachineName: internalAttachment.Machine.Name,
				MachineUUID: internalAttachment.Machine.UUID,
			}
		}

		if retVal.Kind == domainstorage.StorageKindFilesystem &&
			// If the storage kind is Filesystem and we have a mount point set
			// the location to the Filesystem mount point.
			internalAttachment.Filesystem != nil {
			attachment.Location = internalAttachment.Filesystem.MountPoint
		} else if retVal.Kind == domainstorage.StorageKindBlock &&
			// If the storage kind is Block and we have device links from the
			// volume set the location based off of the device link.
			internalAttachment.Volume != nil {
			loc := domainblockdevice.IDLink(internalAttachment.Volume.DeviceNameLinks)
			attachment.Location = loc
		}

		retVal.UnitAttachments = append(retVal.UnitAttachments, attachment)
	}

	return retVal, nil
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

	// We don't have any validation that we run over storage ID's at the moment.
	return s.st.GetStorageInstanceUUIDByID(ctx, storageID)
}
