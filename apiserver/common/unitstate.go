// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/rpc/params"
)

// UnitStateService describes the ability to retrieve and persist
// remote state for informing hook reconciliation.
type UnitStateService interface {
	// SetState persists the input unit state.
	SetState(context.Context, unitstate.UnitState) error
	// GetState returns the internal state of the unit. The return data will be
	// empty if no hook has been run for this unit.
	GetState(ctx context.Context, name coreunit.Name) (unitstate.RetrievedUnitState, error)
}

type UnitStateAPI struct {
	controllerConfigService ControllerConfigService
	unitStateService        UnitStateService

	AccessMachine GetAuthFunc
	accessUnit    GetAuthFunc

	logger corelogger.Logger
}

// NewUnitStateAPI returns a new UnitStateAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewUnitStateAPI(
	controllerConfigService ControllerConfigService,
	unitStateService UnitStateService,
	accessUnit GetAuthFunc,
	logger corelogger.Logger,
) *UnitStateAPI {
	return &UnitStateAPI{
		controllerConfigService: controllerConfigService,
		unitStateService:        unitStateService,
		accessUnit:              accessUnit,
		logger:                  logger,
	}
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) State(ctx context.Context, args params.Entities) (params.UnitStateResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.UnitStateResults{}, errors.Trace(err)
	}

	res := make([]params.UnitStateResult, len(args.Entities))
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		unitState, err := u.unitStateService.GetState(ctx, unitName)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		res[i] = params.UnitStateResult{
			CharmState:    unitState.CharmState,
			UniterState:   unitState.UniterState,
			RelationState: unitState.RelationState,
			StorageState:  unitState.StorageState,
			SecretState:   unitState.SecretState,
		}
	}

	return params.UnitStateResults{Results: res}, nil
}

// SetState sets the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) SetState(ctx context.Context, args params.SetUnitStateArgs) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	/*
		ctrlCfg, err := u.controllerConfigService.ControllerConfig(ctx)
		if err != nil {
			return params.ErrorResults{}, errors.Trace(err)
		}

		TODO (manadart 2024-11-18): Factor these into the transactional
		state setting service method.
		ctrlCfg.MaxCharmStateSize(),
		ctrlCfg.MaxAgentStateSize(),
	*/

	res := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		unitTag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		unitName, err := coreunit.NewName(unitTag.Id())
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		if err := u.unitStateService.SetState(ctx, unitstate.UnitState{
			Name:          unitName,
			CharmState:    arg.CharmState,
			UniterState:   arg.UniterState,
			RelationState: arg.RelationState,
			StorageState:  arg.StorageState,
			SecretState:   arg.SecretState,
		}); err != nil {
			res[i].Error = apiservererrors.ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}
