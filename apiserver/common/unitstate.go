// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/unitstate.go github.com/juju/juju/apiserver/common UnitStateBackend,UnitStateUnit
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/modeloperation.go github.com/juju/juju/state ModelOperation

// UnitStateUnit describes unit-receiver state methods required
// for UnitStateAPI.
type UnitStateBackend interface {
	ApplyOperation(state.ModelOperation) error
	Unit(string) (UnitStateUnit, error)
	ControllerConfig() (controller.Config, error)
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

func (s UnitStateState) ControllerConfig() (controller.Config, error) {
	return s.St.ControllerConfig()
}

type UnitStateAPI struct {
	backend   UnitStateBackend
	resources facade.Resources

	logger loggo.Logger

	AccessMachine GetAuthFunc
	accessUnit    GetAuthFunc
}

// NewExternalUnitStateAPI can be used for API registration.
func NewExternalUnitStateAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *UnitStateAPI {
	return NewUnitStateAPI(UnitStateState{St: st}, resources, authorizer, accessUnit, logger)
}

// NewUnitStateAPI returns a new UnitStateAPI. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewUnitStateAPI(
	backend UnitStateBackend,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit GetAuthFunc,
	logger loggo.Logger,
) *UnitStateAPI {
	return &UnitStateAPI{
		backend:    backend,
		resources:  resources,
		accessUnit: accessUnit,
		logger:     logger,
	}
}

// State returns the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) State(args params.Entities) (params.UnitStateResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.UnitStateResults{}, errors.Trace(err)
	}

	res := make([]params.UnitStateResult, len(args.Entities))
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			res[i].Error = ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = ServerError(ErrPerm)
			continue
		}

		unit, err := u.getUnit(unitTag)
		if err != nil {
			res[i].Error = ServerError(err)
			continue
		}
		unitState, err := unit.State()
		if err != nil {
			res[i].Error = ServerError(err)
			continue
		}

		res[i].CharmState, _ = unitState.CharmState()
		res[i].UniterState, _ = unitState.UniterState()
		res[i].RelationState, _ = unitState.RelationState()
		res[i].StorageState, _ = unitState.StorageState()
		res[i].MeterStatusState, _ = unitState.MeterStatusState()
	}

	return params.UnitStateResults{Results: res}, nil
}

// SetState sets the state persisted by the charm running in this unit
// and the state internal to the uniter for this unit.
func (u *UnitStateAPI) SetState(args params.SetUnitStateArgs) (params.ErrorResults, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	ctrlCfg, err := u.backend.ControllerConfig()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	res := make([]params.ErrorResult, len(args.Args))
	for i, arg := range args.Args {
		unitTag, err := names.ParseUnitTag(arg.Tag)
		if err != nil {
			res[i].Error = ServerError(err)
			continue
		}

		if !canAccess(unitTag) {
			res[i].Error = ServerError(ErrPerm)
			continue
		}

		unit, err := u.getUnit(unitTag)
		if err != nil {
			res[i].Error = ServerError(err)
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
		if arg.MeterStatusState != nil {
			unitState.SetMeterStatusState(*arg.MeterStatusState)
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
			if errors.IsQuotaLimitExceeded(err) {
				logger.Errorf("%s: %v", unitTag, err)
			}
			res[i].Error = ServerError(err)
		}
	}

	return params.ErrorResults{Results: res}, nil
}

func (u *UnitStateAPI) getUnit(tag names.UnitTag) (UnitStateUnit, error) {
	return u.backend.Unit(tag.Id())
}
