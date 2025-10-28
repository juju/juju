// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"time"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreerrors "github.com/juju/juju/core/errors"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// DetachStorage sets the specified storage attachment(s) to Dying, unless they
// are already Dying or Dead. Any associated, persistent storage will remain
// alive. This call can be forced to only remove the attachment. Force will not
// bypass business logic or safety checks.
//
// For long term Juju version reasons this facade endpoint can be used in
// with different compositions. A caller can choose to specify a storage id and
// a unit for which the storage is to be removed from.
//
// Alternatively the caller is free to not provide a specific unit for the
// storage to be detached from. In this case all attachments of the storage
// instances will be removed with the end result being the storage is not used
// by any unit in the model.
//
// NOTE (tlm): As of this writing the Juju client will not supply a unit for the
// detach operation. Other known Juju clients like python-libjuju will supply
// a unit for the removal.
//
// When considering a future redesign of this API this dual execution path
// should be removed with explicit endpoints to support both operations. This
// API also supports batch operations. The implementation is currently such that
// each detach is performed as its own operation. Ideally they should be a
// single atomic operation giving the caller confidence that a half finished
// state is not possible at the end of the call.
func (a *StorageAPI) DetachStorage(
	ctx context.Context,
	args params.StorageDetachmentParams,
) (params.ErrorResults, error) {
	var (
		force    bool
		waitTime time.Duration
	)
	if args.MaxWait != nil && *args.MaxWait < 0 {
		err := errors.Errorf(
			"max wait time cannot be a negative number",
		).Add(coreerrors.NotValid)
		return params.ErrorResults{}, apiservererrors.ServerError(err)
	} else if args.MaxWait != nil {
		waitTime = *args.MaxWait
	}

	if args.Force != nil {
		force = *args.Force
	}

	processStorageAttachmentID := func(
		id params.StorageAttachmentId,
	) error {
		storageTag, err := names.ParseStorageTag(id.StorageTag)
		if err != nil {
			return errors.Errorf(
				"invalid storage tag %q", id.StorageTag,
			).Add(coreerrors.NotValid)
		}

		if id.UnitTag == "" {
			// If the caller did not specify a unit tag, then we detach the
			// storage instance from all units it is attached to. The Juju
			// client does this and never supplied a unit tag.
			return a.detatchStorageInstance(ctx, storageTag.Id(), force, waitTime)
		}

		// If the caller supplied a unit tag then we only detach the storage
		// instance for this unit. We see this value supplied in python-libjuju
		unitTag, err := names.ParseUnitTag(id.UnitTag)
		if err != nil {
			return errors.Errorf(
				"invalid unit tag %q", id.UnitTag,
			).Add(coreerrors.NotValid)
		}

		return a.detachStorageInstanceFromUnit(
			ctx,
			storageTag.Id(),
			coreunit.Name(unitTag.Id()),
			force,
			waitTime,
		)
	}

	result := make([]params.ErrorResult, 0, len(args.StorageIds.Ids))
	for _, attachID := range args.StorageIds.Ids {
		err := processStorageAttachmentID(attachID)
		result = append(result, params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		})
	}

	return params.ErrorResults{Results: result}, nil
}

// detachStorageAttachment takes a single storage attachment uuid to remove
// in the model and actions the removal through the removal service. This func
// acts as a final aggregator of actions to perform for
// [StorageAPI.detachStorageInstanceFromUnit] and
// [StorageAPI.detachStorageAttachment].
//
// The returned errors are guaranteed to have been processed for returning to
// the client. The errors are devoid of context for how the client started the
// operation.
func (a *StorageAPI) detachStorageAttachment(
	ctx context.Context,
	storageAttachment domainstorageprovisioning.StorageAttachmentUUID,
	force bool,
	wait time.Duration,
) error {
	removalUUID, err := a.removalService.RemoveStorageAttachmentFromAliveUnit(
		ctx, storageAttachment, force, wait,
	)

	// Early exit path. Everything below this point can now be considered error
	// handling.
	if err == nil {
		a.logger.Debugf(
			ctx, "storage attachment %q removed with uuid %q",
			storageAttachment, removalUUID,
		)
		return nil
	}

	switch {
	case errors.HasType[applicationerrors.UnitStorageMinViolation](err):
		viErr, _ := errors.AsType[applicationerrors.UnitStorageMinViolation](err)
		return errors.Errorf(
			"removing storage from unit would violate charm storage %q requirements of having minimum %d storage instances",
			viErr.CharmStorageName, viErr.RequiredMinimum,
		).Add(coreerrors.NotValid)
	case errors.Is(err, applicationerrors.UnitNotAlive):
		return errors.New(
			"storage's attached unit must be alive to remove",
		).Add(coreerrors.NotValid)
	case errors.Is(err, storageerrors.StorageAttachmentNotFound):
		// The storage attachment has already been removed. We had already
		// resolved that it existed above and so we can safely ignore this
		// error.
		return nil
	default:
		return errors.Errorf("removing storage: %w", err)
	}
}

// detachStorageInstanceFromUnit detaches exactly one storage instance for a
// request from the supplied unit. It expects that the caller has processed the
// supplied tags and can now provide string values representing the entities.
func (a *StorageAPI) detachStorageInstanceFromUnit(
	ctx context.Context,
	storageID string,
	unitName coreunit.Name,
	force bool,
	wait time.Duration,
) error {
	unitUUID, err := a.applicationService.GetUnitUUID(ctx, unitName)
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return errors.Errorf("invalid unit name %q", unitName).Add(
			coreerrors.NotValid,
		)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf("unit %q does not exist", unitName).Add(coreerrors.NotFound)
	case err != nil:
		return errors.Errorf(
			"getting unit uuid for unit name %q: %w", unitName, err,
		)
	}

	storageInstanceUUID, err := a.storageService.GetStorageInstanceUUIDForID(
		ctx, storageID,
	)
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(
			coreerrors.NotFound,
		)
	case err != nil:
		return errors.Errorf(
			"getting storage instance uuid for storage id %q: %w", storageID, err,
		)
	}

	storageAttachmentUUID, err := a.storageService.
		GetStorageAttachmentUUIDForStorageInstanceAndUnit(
			ctx, storageInstanceUUID, unitUUID,
		)
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(coreerrors.NotFound)
	case errors.Is(err, storageerrors.StorageAttachmentNotFound):
		return errors.Errorf(
			"storage %q is not attached to unit %q", storageID, unitName,
		).Add(coreerrors.NotFound)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf("unit %q does not exist", unitName).Add(coreerrors.NotFound)
	case err != nil:
		return errors.Errorf(
			"getting storage attachment uuid for storage %q attached to unit %q: %w",
			storageID, unitName, err,
		)
	}

	a.logger.Debugf(
		ctx, "detaching storage %q from unit %q",
		storageID, unitName,
	)
	err = a.detachStorageAttachment(ctx, storageAttachmentUUID, force, wait)
	if err != nil {
		return errors.Errorf(
			"detaching storage %q on unit %q: %w", storageID, unitName, err,
		)
	}
	return nil
}

// detachStorageInstance detaches a storage instance from all units that it is
// attached to. It expects that the caller has processed the supplied tags and
// can now provide string values representing the entities.
func (a *StorageAPI) detatchStorageInstance(
	ctx context.Context,
	storageID string,
	force bool,
	wait time.Duration,
) error {
	storageInstanceUUID, err := a.storageService.GetStorageInstanceUUIDForID(
		ctx, storageID,
	)
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(
			coreerrors.NotFound,
		)
	case err != nil:
		return errors.Errorf(
			"getting storage instance uuid for storage id %q: %w", storageID, err,
		)
	}

	storageAttachmentUUIDs, err := a.storageService.
		GetStorageInstanceAttachments(ctx, storageInstanceUUID)
	// We purposely ignore not valid errors for the uuids supplied. We have
	// received these uuids from the domain and not the caller so they can
	// safely be considered valid.
	switch {
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return errors.Errorf("storage %q does not exist", storageID).Add(coreerrors.NotFound)
	case err != nil:
		return errors.Errorf(
			"getting storage attachments for storage %q: %w",
			storageID, err,
		)
	}

	a.logger.Debugf(
		ctx, "detaching storage %q from %d attachments",
		storageID, len(storageAttachmentUUIDs),
	)
	for i, saUUID := range storageAttachmentUUIDs {
		err := a.detachStorageAttachment(ctx, saUUID, force, wait)
		if err != nil {
			return errors.Errorf(
				"detaching storage %q attachment %d: %w",
				storageID, i, err,
			)
		}
	}
	return nil
}
