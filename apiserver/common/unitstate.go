// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/unitstate.go github.com/juju/juju/apiserver/common UnitStateBackend,UnitStateUnit
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modeloperation.go github.com/juju/juju/state ModelOperation

// UnitStateBackend describes unit-receiver state methods required
// for UnitStateAPI.
type UnitStateBackend interface {
	ApplyOperation(state.ModelOperation) error
	Unit(string) (UnitStateUnit, error)
}

// UnitStateUnit describes unit-receiver state methods required
// for UnitStateAPI.
type UnitStateUnit interface {
	SetStateOperation(*state.UnitState, state.UnitStateSizeLimits) state.ModelOperation
	State() (*state.UnitState, error)
}

// UnitStateState implements the UnitStateBackend indirection
// over state.State.
type UnitStateState struct {
	St *state.State
}

func (s UnitStateState) ApplyOperation(op state.ModelOperation) error {
	return s.St.ApplyOperation(op)
}

func (s UnitStateState) Unit(name string) (UnitStateUnit, error) {
	return s.St.Unit(name)
}

// UnitStateService describes the ability to retrieve and persist
// remote state for informing hook reconciliation.
type UnitStateService interface {
	// GetUnitUUIDForName returns the UUID corresponding to the input unit name.
	GetUnitUUIDForName(ctx context.Context, name string) (string, error)
	// SetState persists the input unit state.
	SetState(context.Context, unitstate.UnitState) error
	// GetState returns the internal state of the unit. The return data will be
	// empty if no hook has been run for this unit.
	GetState(ctx context.Context, uuid string) (unitstate.RetrievedUnitState, error)
}

type UnitStateAPI struct {
	controllerConfigService ControllerConfigService
	unitStateService        UnitStateService

	backend   UnitStateBackend
	resources facade.Resources

	logger corelogger.Logger

	AccessMachine GetAuthFunc
	accessUnit    GetAuthFunc
}

// NewExternalUnitStateAPI can be used for API registration.
func NewExternalUnitStateAPI(
	controllerConfigService ControllerConfigService,
	unitStateService UnitStateService,
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit GetAuthFunc,
	logger corelogger.Logger,
) *UnitStateAPI {
	return NewUnitStateAPI(
		controllerConfigService, unitStateService, UnitStateState{St: st}, resources, authorizer, accessUnit, logger)
}

// NewUnitStateAPI returns a new UnitStateAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewUnitStateAPI(
	controllerConfigService ControllerConfigService,
	unitStateService UnitStateService,
	backend UnitStateBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit GetAuthFunc,
	logger corelogger.Logger,
) *UnitStateAPI {
	return &UnitStateAPI{
		controllerConfigService: controllerConfigService,
		unitStateService:        unitStateService,
		backend:                 backend,
		resources:               resources,
		accessUnit:              accessUnit,
		logger:                  logger,
	}
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) State(ctx context.Context, args params.Entities) (params.UnitStateResults, error) {
	canAccess, err := u.accessUnit()
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
		unitUUID, err := u.unitStateService.GetUnitUUIDForName(ctx, unitTag.Id())
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		unitState, err := u.unitStateService.GetState(ctx, unitUUID)
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
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	ctrlCfg, err := u.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

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

		unit, err := u.getUnit(unitTag)
		if err != nil {
			res[i].Error = apiservererrors.ServerError(err)
			continue
		}

		unitState := state.NewUnitState()
		if arg.CharmState != nil {
			unitState.SetCharmState(*arg.CharmState)
		}
		if arg.UniterState != nil {
			unitState.SetUniterState(*arg.UniterState)
		}
		if arg.RelationState != nil {
			unitState.SetRelationState(*arg.RelationState)
		}
		if arg.StorageState != nil {
			unitState.SetStorageState(*arg.StorageState)
		}
		if arg.SecretState != nil {
			unitState.SetSecretState(*arg.SecretState)
		}

		ops := unit.SetStateOperation(
			unitState,
			state.UnitStateSizeLimits{
				MaxCharmStateSize: ctrlCfg.MaxCharmStateSize(),
				MaxAgentStateSize: ctrlCfg.MaxAgentStateSize(),
			},
		)
		if err = u.backend.ApplyOperation(ops); err != nil {
			// Log quota-related errors to aid operators
			if errors.Is(err, errors.QuotaLimitExceeded) {
				logger.Errorf("%s: %v", unitTag, err)
			}
			res[i].Error = apiservererrors.ServerError(err)
		}

		if err := u.unitStateService.SetState(ctx, unitstate.UnitState{
			Name:          unitTag.Id(),
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

func (u *UnitStateAPI) getUnit(tag names.UnitTag) (UnitStateUnit, error) {
	return u.backend.Unit(tag.Id())
}
