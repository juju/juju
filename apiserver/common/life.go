// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// LifeGetter implements a common Life method for use by various facades.
type LifeGetter struct {
	st                 state.EntityFinder
	applicationService LifeGetterApplicationService
	getCanRead         GetAuthFunc
}

// LifeGetterApplicationService provides application domain service methods for
// getting the life of applications and units.
type LifeGetterApplicationService interface {
	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
	GetUnitLife(ctx context.Context, unitName coreunit.Name) (life.Value, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}

// NewLifeGetter returns a new LifeGetter. The GetAuthFunc will be used on
// each invocation of Life to determine current permissions.
func NewLifeGetter(
	st state.EntityFinder,
	getCanRead GetAuthFunc,
	applicationService LifeGetterApplicationService,
) *LifeGetter {
	return &LifeGetter{
		st:                 st,
		getCanRead:         getCanRead,
		applicationService: applicationService,
	}
}

// OneLife returns the life of the specified entity.
func (lg *LifeGetter) OneLife(ctx context.Context, tag names.Tag) (life.Value, error) {
	switch tag := tag.(type) {
	case names.UnitTag:
		if lg.applicationService == nil {
			return "", errors.NotSupportedf("unit life getting")
		}
		unitLife, err := lg.applicationService.GetUnitLife(ctx, coreunit.Name(tag.Id()))
		if errors.Is(err, applicationerrors.UnitNotFound) {
			err = errors.NotFoundf("unit %q", tag.Id())
		} else if err != nil {
			return "", errors.Trace(err)
		}
		return unitLife, nil
	case names.ApplicationTag:
		if lg.applicationService == nil {
			return "", errors.NotSupportedf("application life getting")
		}
		appLife, err := lg.applicationService.GetApplicationLifeByName(ctx, tag.Id())
		if errors.Is(err, applicationerrors.ApplicationNotFound) {
			err = errors.NotFoundf("application %q", tag.Id())
		} else if err != nil {
			return "", errors.Trace(err)
		}
		return appLife, nil
	}

	entity0, err := lg.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.Lifer)
	if !ok {
		return "", apiservererrors.NotSupportedError(tag, "life cycles")
	}
	return life.Value(entity.Life().String()), nil
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
		err = apiservererrors.ErrPerm
		if canRead(tag) {
			result.Results[i].Life, err = lg.OneLife(ctx, tag)
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
