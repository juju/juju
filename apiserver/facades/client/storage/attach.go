// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Attach attaches existing storage instances to units.
func (a *StorageAPI) Attach(ctx context.Context, args params.StorageAttachmentIds) (params.ErrorResults, error) {
	if err := a.checkCanWrite(ctx); err != nil {
		return params.ErrorResults{}, errors.Capture(err)
	}

	// Check if changes are allowed and the operation may proceed.
	if err := a.blockChecker.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Capture(err)
	}

	result := make([]params.ErrorResult, len(args.Ids))
	for i, one := range args.Ids {
		err := a.attachOneStorage(ctx, one)
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (a *StorageAPI) attachOneStorage(ctx context.Context, one params.StorageAttachmentId) error {
	u, err := names.ParseUnitTag(one.UnitTag)
	if err != nil {
		return apiservererrors.ParamsErrorf(params.CodeNotValid, "invalid unit tag")
	}

	unitName := coreunit.Name(u.Id())
	unitUUID, err := a.applicationService.GetUnitUUID(ctx, unitName)
	switch {
	case errors.Is(err, coreunit.InvalidUnitName):
		return apiservererrors.ParamsErrorf(params.CodeNotValid,
			"invalid unit name %q", unitName)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return apiservererrors.ParamsErrorf(params.CodeNotFound,
			"unit %q does not exist", unitName)
	case err != nil:
		return errors.Errorf(
			"getting unit uuid for unit name %q: %w", unitName, err,
		)
	}

	storageTag, err := names.ParseStorageTag(one.StorageTag)
	if err != nil {
		return apiservererrors.ParamsErrorf(params.CodeNotValid, "invalid storage tag")
	}
	storageUUID, err := a.storageService.GetStorageInstanceUUIDForID(ctx, storageTag.Id())
	if errors.Is(err, storageerrors.StorageInstanceNotFound) {
		return apiservererrors.ParamsErrorf(params.CodeNotFound, "storage %q does not exist", storageTag.Id())
	} else if err != nil {
		return errors.Errorf(
			"getting storage instance uuid for storage id %q: %w",
			storageTag.Id(), err,
		)
	}

	err = a.applicationService.AttachStorageToUnit(ctx, storageUUID, unitUUID)
	err = handleAttachStorageInstanceToUnitError(err, unitName, storageTag.Id())
	return err
}

// handleAttachStorageInstanceToUnitError maps domain errors from
// AttachStorageToUnit into appropriate API errors. If no specific handler
// exists the original error is returned.
func handleAttachStorageInstanceToUnitError(
	err error, unitName coreunit.Name, storageID string,
) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, applicationerrors.UnitNotFound):
		return apiservererrors.ParamsErrorf(
			params.CodeNotFound,
			"unit %q does not exist", unitName,
		)
	case errors.Is(err, applicationerrors.UnitNotAlive):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"unit %q is not alive", unitName,
		)
	case errors.Is(err, storageerrors.StorageInstanceNotFound):
		return apiservererrors.ParamsErrorf(
			params.CodeNotFound,
			"storage %q not found", storageID,
		)
	case errors.Is(err, storageerrors.StorageInstanceNotAlive):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q is not alive", storageID,
		)
	case errors.Is(err, applicationerrors.StorageNameNotSupported):
		return apiservererrors.ParamsErrorf(
			params.CodeNotSupported,
			"storage %q not supported by the charm of unit %q",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceCharmNameMismatch):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q was created for a different charm than unit %q",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceKindNotValidForCharmStorageDefinition):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q kind is not compatible with the charm storage definition of unit %q",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceSizeNotValidForCharmStorageDefinition):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q size does not meet the charm storage minimum size requirement of unit %q",
			storageID, unitName,
		)
	case errors.HasType[applicationerrors.StorageCountLimitExceeded](err):
		limitErr, _ := errors.AsType[applicationerrors.StorageCountLimitExceeded](err)
		if limitErr.Maximum != nil && limitErr.Requested > *limitErr.Maximum {
			return apiservererrors.ParamsErrorf(
				params.CodeNotValid,
				"attaching storage %q would exceed the maximum count of %d"+
					" for storage definition %q of unit %q",
				storageID, *limitErr.Maximum, limitErr.StorageName, unitName,
			)
		}
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"attaching storage %q to unit %q: %v",
			storageID, unitName, limitErr,
		)
	case errors.Is(err, applicationerrors.StorageInstanceAlreadyAttachedToUnit):
		return apiservererrors.ParamsErrorf(
			params.CodeAlreadyExists,
			"storage %q is already attached to unit %q",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceAttachSharedAccessNotSupported):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q already has attachments but the charm storage definition of unit %q does not support shared access",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceUnexpectedAttachments):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q attachments changed while attaching to unit %q, please retry",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.StorageInstanceAttachMachineOwnerMismatch):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"storage %q is bound to a different machine than unit %q",
			storageID, unitName,
		)
	case errors.Is(err, applicationerrors.UnitCharmChanged):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"unit %q charm changed during storage attachment, please retry",
			unitName,
		)
	case errors.Is(err, applicationerrors.UnitMachineChanged):
		return apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"unit %q machine changed during storage attachment, please retry",
			unitName,
		)
	}
	return err
}
