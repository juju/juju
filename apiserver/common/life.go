// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
)

// ApplicationService defines the methods required to get the life of a unit.
type ApplicationService interface {
	GetUnitLife(ctx context.Context, name unit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}

// LifeGetter implements a common Life method for use by various facades.
type LifeGetter struct {
	applicationService ApplicationService
	machineService     MachineService
	getCanRead         GetAuthFunc
	logger             corelogger.Logger
}

// NewLifeGetter returns a new LifeGetter. The GetAuthFunc will be used on
// each invocation of Life to determine current permissions.
func NewLifeGetter(
	applicationService ApplicationService,
	machineService MachineService,
	getCanRead GetAuthFunc,
	logger corelogger.Logger,
) *LifeGetter {
	return &LifeGetter{
		applicationService: applicationService,
		machineService:     machineService,
		getCanRead:         getCanRead,
		logger:             logger,
	}
}

// Life returns the life status of every supplied entity, where available.
func (lg *LifeGetter) Life(ctx context.Context, args params.Entities) (params.LifeResults, error) {
	result := params.LifeResults{
		Results: make([]params.LifeResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := lg.getCanRead(ctx)
	if err != nil {
		return params.LifeResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !canRead(tag) {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		life, err := lg.OneLife(ctx, tag)
		result.Results[i].Life = life
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// OneLife returns the life of the specified entity.
func (lg *LifeGetter) OneLife(ctx context.Context, tag names.Tag) (life.Value, error) {
	switch t := tag.(type) {
	case names.MachineTag:
		life, err := lg.machineService.GetMachineLife(ctx, machine.Name(t.Id()))
		if errors.Is(err, machineerrors.NotProvisioned) {
			return "", errors.NotProvisionedf("machine %s", t.Id())
		} else if errors.Is(err, machineerrors.MachineNotFound) {
			return "", errors.NotFoundf("machine %s", t.Id())
		}
		return life, errors.Trace(err)

	case names.UnitTag:
		life, err := lg.applicationService.GetUnitLife(ctx, unit.Name(t.Id()))
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return "", errors.NotFoundf("unit %s", t.Id())
		}
		return life, errors.Trace(err)

	case names.ApplicationTag:
		if lg.applicationService == nil {
			return "", errors.NotSupportedf("application life getting")
		}
		appLife, err := lg.applicationService.GetApplicationLifeByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			return "", errors.NotFoundf("application %q", tag.Id())
		} else if err != nil {
			return "", errors.Trace(err)
		}
		return appLife, nil
	default:
		lg.logger.Criticalf(ctx, "OneLife called with unsupported tag %s", tag)
		return "", errors.Errorf("unsupported tag %s", tag)
	}
}
