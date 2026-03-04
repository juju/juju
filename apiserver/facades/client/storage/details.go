// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"slices"
	"time"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/names/v6"
)

// getOneStorageDetails processes and retreives the storage details for a single
// storage instance based off of the storage id provided. The returned value is
// a [params.StorageDetailsResult] with either the Result or Error field
// populated.
//
// Should an error occur processing the supplied values from the caller of the
// facade this will result in an empty [params.StorageDetailsResult] an error.
// This case indicates the any further processing of the facade request should
// stop.
func (a *StorageAPI) getOneStorageDetails(
	ctx context.Context, storageID string,
) (params.StorageDetailsResult, error) {
	uuid, err := a.storageService.GetStorageInstanceUUIDForID(ctx, storageID)
	switch {
	case errors.Is(err, coreerrors.NotValid):
		return params.StorageDetailsResult{}, apiservererrors.ParamsErrorf(
			params.CodeNotValid, "invalid storage id supplied",
		)
	case errors.Is(err, domainstorageerrors.StorageInstanceNotFound):
		return params.StorageDetailsResult{
			Error: apiservererrors.ParamsErrorf(
				params.CodeNotFound, "storage instance %q not found", storageID,
			),
		}, nil
	case err != nil:
		return params.StorageDetailsResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	info, err := a.storageService.GetStorageInstanceInfo(ctx, uuid)
	switch {
	case errors.Is(err, domainstorageerrors.StorageInstanceNotFound):
		return params.StorageDetailsResult{
			Error: apiservererrors.ParamsErrorf(
				params.CodeNotFound, "storage instance %q not found", storageID,
			),
		}, nil
	case err != nil:
		return params.StorageDetailsResult{
			Error: apiservererrors.ServerError(err),
		}, nil
	}

	var kind params.StorageKind
	switch info.Kind {
	case domainstorage.StorageKindBlock:
		kind = params.StorageKindBlock
	case domainstorage.StorageKindFilesystem:
		kind = params.StorageKindFilesystem
	default:
		kind = params.StorageKindUnknown
	}

	life, err := info.Life.Value()
	if err != nil {
		a.logger.Warningf(
			ctx,
			"unable to translate life value %d to params for storage instance %q: %s",
			info.Life,
			uuid,
			err.Error(),
		)
		return params.StorageDetailsResult{
			Error: apiservererrors.ParamsErrorf(
				"", "unknown life value for storage instance %q", storageID,
			),
		}, nil
	}

	var unitOwnerTagStr string
	if info.UnitOwner != nil {
		// Storage instance has unit owner.
		unitOwnerTagStr = names.NewUnitTag(info.UnitOwner.Name).String()
	}

	var status params.EntityStatus
	if info.Kind == domainstorage.StorageKindFilesystem && info.FilesystemStatus != nil {
		status.Data = info.FilesystemStatus.Data
		status.Info = info.FilesystemStatus.Message
		status.Since = info.FilesystemStatus.Since
		status.Status = info.FilesystemStatus.Status
	} else if info.VolumeStatus != nil {
		status.Data = info.VolumeStatus.Data
		status.Info = info.VolumeStatus.Message
		status.Since = info.VolumeStatus.Since
		status.Status = info.VolumeStatus.Status
	}
	if status.Since == nil {
		// We have to set a non nil value for the status since value otherwise
		// the Juju client will panic.
		zeroTime := time.UnixMicro(0).UTC()
		status.Since = &zeroTime
	}

	storageTagStr := names.NewStorageTag(storageID).String()
	details := params.StorageDetails{
		Attachments: map[string]params.StorageAttachmentDetails{},
		Status:      status,
		StorageTag:  storageTagStr,
		Kind:        kind,
		Life:        life,
		Persistent:  info.Persistent,
		OwnerTag:    unitOwnerTagStr,
	}

	for _, unitAttachment := range info.UnitAttachments {
		unitTagStr := names.NewUnitTag(unitAttachment.UnitName).String()
		var machineTagStr string
		if unitAttachment.MachineAttachment != nil {
			machineTagStr = names.NewMachineTag(
				unitAttachment.MachineAttachment.MachineName,
			).String()
		}

		attachmentLife, err := unitAttachment.Life.Value()
		if err != nil {
			if err != nil {
				a.logger.Warningf(
					ctx,
					"unable to translate life value %d to params for storage instance attachment %q: %s",
					unitAttachment.Life,
					unitAttachment.UUID,
					err.Error(),
				)
				return params.StorageDetailsResult{
					Error: apiservererrors.ParamsErrorf(
						"", "unknown life value for storage instance %q attachment", storageID,
					),
				}, nil
			}
		}

		details.Attachments[unitTagStr] = params.StorageAttachmentDetails{
			Life:       attachmentLife,
			Location:   unitAttachment.Location,
			MachineTag: machineTagStr,
			StorageTag: storageTagStr,
			UnitTag:    unitTagStr,
		}
	}

	return params.StorageDetailsResult{Result: &details}, nil
}

// StorageDetails retrieves and returns detailed information about desired
// storage identified by supplied tags. If specified storage cannot be
// retrieved, individual error is returned instead of storage information.
func (a *StorageAPI) StorageDetails(ctx context.Context, entities params.Entities) (params.StorageDetailsResults, error) {
	if err := a.checkCanRead(ctx); err != nil {
		return params.StorageDetailsResults{}, err
	}

	// Make a list of the ids we received and the ids for which information
	// needs to be fetched.
	storageIDsToGet := make([]string, 0, len(entities.Entities))
	storageIDsReceived := make([]string, 0, len(entities.Entities))
	for i, entity := range entities.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			// Don't process the bad tag value any further. Use the index in the
			// slice to indicate which tag was bad.
			return params.StorageDetailsResults{}, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "invalid storage entity tag %d", i,
			)
		}

		entityTagKind := tag.Kind()
		if entityTagKind != names.StorageTagKind {
			return params.StorageDetailsResults{}, apiservererrors.ParamsErrorf(
				params.CodeNotValid, "tag kind %q not supported", entityTagKind,
			)
		}

		storageIDsToGet = append(storageIDsToGet, tag.Id())
		storageIDsReceived = append(storageIDsReceived, tag.Id())
	}

	// Because we process each entity individually we must de-dupe the id's to
	// not thrash the service layer for no reason. Results for each unique id
	// are stored in a map for building the final result.
	slices.Sort(storageIDsToGet)
	storageIDsToGet = slices.Compact(storageIDsToGet)
	results := make(map[string]params.StorageDetailsResult, len(storageIDsToGet))

	for _, id := range storageIDsToGet {
		result, err := a.getOneStorageDetails(ctx, id)
		if err != nil {
			return params.StorageDetailsResults{}, err
		}
		results[id] = result
	}

	retVal := make([]params.StorageDetailsResult, 0, len(storageIDsReceived))
	for _, id := range storageIDsReceived {
		retVal = append(retVal, results[id])
	}

	return params.StorageDetailsResults{Results: retVal}, nil
}
