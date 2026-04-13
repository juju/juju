// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	applicationservice "github.com/juju/juju/domain/application/service"
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

	if api.modelType == model.CAAS {
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

	if api.modelType == model.CAAS {
		// In a CAAS model, there are no machines for
		// units to be assigned to.
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

	// Parse storage tags in AttachStorage.
	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf(
			"AttachStorage is non-empty, but NumUnits is %d", args.NumUnits,
		).Add(coreerrors.NotValid)
	}
	attachStorage := make([]names.StorageTag, len(args.AttachStorage))
	for i, tagString := range args.AttachStorage {
		tag, err := names.ParseStorageTag(tagString)
		if err != nil {
			return nil, errors.Capture(err)
		}
		attachStorage[i] = tag
	}

	return api.addUnits(
		ctx,
		args.ApplicationName,
		args.NumUnits,
		args.Placement,
		attachStorage,
	)
}

// addUnits starts n units of the given application using the specified placement
// directives to allocate the machines.
func (api *APIBase) addUnits(
	ctx context.Context,
	appName string,
	n int,
	placement []*instance.Placement,
	attachStorage []names.StorageTag,
) ([]coreunit.Name, error) {
	units := make([]coreunit.Name, 0, n)

	// TODO what do we do if we fail half-way through this process?
	for i := range n {
		var unitPlacement *instance.Placement
		if i < len(placement) {
			unitPlacement = placement[i]
		}

		unitArg := applicationservice.AddUnitArg{
			Placement: unitPlacement,
		}

		var unitNames []coreunit.Name
		var err error
		if api.modelType == coremodel.CAAS {
			unitNames, err = api.applicationService.AddCAASUnits(ctx, appName, unitArg)
		} else {
			unitNames, _, err = api.applicationService.AddIAASUnits(ctx, appName, applicationservice.AddIAASUnitArg{
				AddUnitArg: unitArg,
			})
		}
		if err != nil {
			return nil, errors.Errorf("adding unit to application %q: %w", appName, err)
		}
		units = append(units, unitNames...)
	}

	return units, nil
}

// makeIAASAddUnitArgs creates [n] add iaas unit args taking placement and
// existing storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeIAASAddUnitArgs(
	n int,
	placement []*instance.Placement,
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

// makeCAASAddUnitArgs creates [n] add caas unit args taking placement and existing
// storage instances to attach to each unit. Both [placement] and
// [storageInstancesToAttach] are position dependent, as in, the zeroth unit to
// be added takes the zeroth placement and storage.
func makeCAASAddUnitArgs(
	n int,
	placement []*instance.Placement,
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
	placement []*instance.Placement,
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
