// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	coreinstance "github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	domainapplicationservice "github.com/juju/juju/domain/application/service"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// AddUnits adds a given number of units to an application.
func (api *APIBase) AddUnits(
	ctx context.Context, args params.AddApplicationUnits,
) (params.AddApplicationUnitsResults, error) {
	if err := api.checkCanWrite(ctx); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Capture(err)
	}
	if err := api.check.ChangeAllowed(ctx); err != nil {
		return params.AddApplicationUnitsResults{}, errors.Capture(err)
	}

	if api.modelType == coremodel.CAAS {
		return params.AddApplicationUnitsResults{}, errors.Errorf(
			"adding units to a container-based model is not supported",
		).Add(coreerrors.NotSupported)
	}

	units, err := api.addApplicationUnits(ctx, args)
	if err != nil {
		return params.AddApplicationUnitsResults{}, errors.Capture(err)
	}

	return params.AddApplicationUnitsResults{
		Units: transform.Slice(units, coreunit.Name.String),
	}, nil
}

// addApplicationUnits adds a given number of units to an application.
func (api *APIBase) addApplicationUnits(
	ctx context.Context, args params.AddApplicationUnits,
) ([]coreunit.Name, error) {
	if args.NumUnits < 1 {
		return nil, errors.New(
			"must add at least one unit",
		).Add(coreerrors.NotValid)
	}

	if api.modelType == coremodel.CAAS {
		// In a CAAS model, there are no machines for units to be assigned to.
		if len(args.AttachStorage) > 0 {
			return nil, errors.Errorf(
				"AttachStorage may not be specified for %s models",
				api.modelType,
			).Add(coreerrors.NotSupported)
		}
		if len(args.Placement) > 1 {
			return nil, errors.Errorf(
				"only 1 placement directive is supported for %s models, got %d",
				api.modelType,
				len(args.Placement),
			).Add(coreerrors.NotSupported)
		}
	}

	attachStorageIDs := make([]string, 0, len(args.AttachStorage))
	seenAttachStorageIDs := make(map[string]struct{}, len(args.AttachStorage))
	for _, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if _, exists := seenAttachStorageIDs[tag.Id()]; exists {
			continue
		}
		seenAttachStorageIDs[tag.Id()] = struct{}{}
		attachStorageIDs = append(attachStorageIDs, tag.Id())
	}

	// TODO(storage): allow attaching storage to more than one new unit.
	if len(attachStorageIDs) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf(
			"AttachStorage is non-empty, but NumUnits is %d", args.NumUnits,
		).Add(coreerrors.NotValid)
	}

	storageInstanceUUIDs, err := api.storageService.GetStorageInstanceUUIDsByIDs(
		ctx, attachStorageIDs)
	if err != nil {
		return nil, errors.Errorf("getting storage instance UUIDs: %w", err)
	}

	storageUUIDsToAttach := make([]domainstorage.StorageInstanceUUID, 0,
		len(attachStorageIDs))
	for _, storageID := range attachStorageIDs {
		storageUUID, ok := storageInstanceUUIDs[storageID]
		if !ok {
			return nil, errors.Errorf(
				"storage instance %q does not exist", storageID,
			).Add(coreerrors.NotFound)
		}
		storageUUIDsToAttach = append(storageUUIDsToAttach, storageUUID)
	}

	storageInstancesToAttach := [][]domainstorage.StorageInstanceUUID{
		storageUUIDsToAttach,
	}

	var unitNames []coreunit.Name
	if api.modelType == coremodel.CAAS {
		addUnitArgs := makeCAASAddUnitArgs(
			args.NumUnits,
			args.Placement,
			storageInstancesToAttach,
		)
		unitNames, err = api.applicationService.AddCAASUnits(
			ctx, args.ApplicationName, addUnitArgs...)
	} else {
		addUnitArgs := makeIAASAddUnitArgs(
			args.NumUnits,
			args.Placement,
			storageInstancesToAttach,
		)
		unitNames, _, err = api.applicationService.AddIAASUnits(
			ctx, args.ApplicationName, addUnitArgs...)
	}
	if errors.Is(err, domainapplicationerrors.ApplicationNotFound) {
		return nil, errors.Errorf(
			"application %q not found", args.ApplicationName,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"adding %d units to application %q: %w",
			args.NumUnits, args.ApplicationName, err,
		)
	}

	return unitNames, nil
}

// makeIAASAddUnitArgs creates [n] add iaas unit args taking placement and
// existing storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeIAASAddUnitArgs(
	n int,
	placement []*coreinstance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddIAASUnitArg {
	retVal := make([]domainapplicationservice.AddIAASUnitArg, 0, n)
	auas := makeAddUnitArgs(n, placement, storageInstancesToAttach)
	for _, aua := range auas {
		val := domainapplicationservice.AddIAASUnitArg{
			AddUnitArg: aua,
		}
		retVal = append(retVal, val)
	}
	return retVal
}

// makeCAASAddUnitArgs creates [n] add caas unit args taking placement and
// existing storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeCAASAddUnitArgs(
	n int,
	placement []*coreinstance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddUnitArg {
	return makeAddUnitArgs(n, placement, storageInstancesToAttach)
}

// makeAddUnitArgs creates [n] add unit args taking placement and existing
// storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeAddUnitArgs(
	n int,
	placement []*coreinstance.Placement,
	storageInstancesToAttach [][]domainstorage.StorageInstanceUUID,
) []domainapplicationservice.AddUnitArg {
	retVal := make([]domainapplicationservice.AddUnitArg, 0, n)
	for i := range n {
		val := domainapplicationservice.AddUnitArg{}
		if i < len(placement) {
			val.Placement = placement[i]
		}
		if i < len(storageInstancesToAttach) {
			val.StorageInstancesToAttach = storageInstancesToAttach[i]
		}
		retVal = append(retVal, val)
	}
	return retVal
}
