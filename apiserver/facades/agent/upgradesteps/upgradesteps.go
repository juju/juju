// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UpgradeStepsV2 defines the methods on the version 2 facade for the
// upgrade steps API endpoint.
type UpgradeStepsV2 interface {
	UpgradeStepsV1
	WriteAgentState(context.Context, params.SetUnitStateArgs) (params.ErrorResults, error)
}

// UpgradeStepsV1 defines the methods on the version 2 facade for the
// upgrade steps API endpoint.
type UpgradeStepsV1 interface {
	ResetKVMMachineModificationStatusIdle(context.Context, params.Entity) (params.ErrorResult, error)
}

// Logger represents the logging methods used by the upgrade steps.
type Logger interface {
	Criticalf(string, ...any)
	Warningf(string, ...any)
}

// UpgradeStepsAPI implements version 2 of the Upgrade Steps API,
// which adds WriteUniterState.
type UpgradeStepsAPI struct {
	st                 UpgradeStepsState
	ctrlConfigGetter   ControllerConfigGetter
	resources          facade.Resources
	authorizer         facade.Authorizer
	getMachineAuthFunc common.GetAuthFunc
	getUnitAuthFunc    common.GetAuthFunc
	logger             Logger
}

// UpgradeStepsAPIV1 implements version 1 of the Upgrade Steps API.
type UpgradeStepsAPIV1 struct {
	*UpgradeStepsAPI
}

var _ UpgradeStepsV2 = (*UpgradeStepsAPI)(nil)

func NewUpgradeStepsAPI(
	st UpgradeStepsState,
	ctrlConfigGetter ControllerConfigGetter,
	resources facade.Resources,
	authorizer facade.Authorizer,
	logger Logger,
) (*UpgradeStepsAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() && !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}

	getMachineAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	getUnitAuthFunc := common.AuthFuncForTagKind(names.UnitTagKind)
	return &UpgradeStepsAPI{
		st:                 st,
		ctrlConfigGetter:   ctrlConfigGetter,
		resources:          resources,
		authorizer:         authorizer,
		getMachineAuthFunc: getMachineAuthFunc,
		getUnitAuthFunc:    getUnitAuthFunc,
		logger:             logger,
	}, nil
}

// ResetKVMMachineModificationStatusIdle sets the modification status
// of a kvm machine to idle if it is in an error state before upgrade.
// Related to lp:1829393.
func (api *UpgradeStepsAPI) ResetKVMMachineModificationStatusIdle(ctx context.Context, arg params.Entity) (params.ErrorResult, error) {
	var result params.ErrorResult
	canAccess, err := api.getMachineAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}

	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	m, err := api.getMachine(canAccess, mTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	if m.ContainerType() != instance.KVM {
		// noop
		return result, nil
	}

	modStatus, err := m.ModificationStatus()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	if modStatus.Status == status.Error {
		err = m.SetModificationStatus(status.StatusInfo{Status: status.Idle})
		result.Error = apiservererrors.ServerError(err)
	}

	return result, nil
}

// WriteAgentState writes the agent state for the set of units provided. This
// call presently deals with the state for the unit agent.
func (api *UpgradeStepsAPI) WriteAgentState(ctx context.Context, args params.SetUnitStateArgs) (params.ErrorResults, error) {
	ctrlCfg, err := api.ctrlConfigGetter.ControllerConfig(ctx)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}

	for i, data := range args.Args {
		canAccess, err := api.getUnitAuthFunc()
		if err != nil {
			return results, errors.Trace(err)
		}
		uTag, err := names.ParseUnitTag(data.Tag)
		if err != nil {
			return results, errors.Trace(err)
		}
		u, err := api.getUnit(canAccess, uTag)
		if err != nil {
			api.logger.Criticalf("failed to get unit %q: %s", uTag, err)
			return results, errors.Trace(err)
		}
		us := state.NewUnitState()
		if data.UniterState != nil {
			us.SetUniterState(*data.UniterState)
		} else {
			api.logger.Warningf("no uniter state provided for %q", uTag)
		}
		if data.RelationState != nil {
			us.SetRelationState(*data.RelationState)
		}
		if data.StorageState != nil {
			us.SetStorageState(*data.StorageState)
		}

		op := u.SetStateOperation(
			us,
			state.UnitStateSizeLimits{
				MaxCharmStateSize: ctrlCfg.MaxCharmStateSize(),
				MaxAgentStateSize: ctrlCfg.MaxAgentStateSize(),
			},
		)
		results.Results[i].Error = apiservererrors.ServerError(api.st.ApplyOperation(op))
	}

	return results, nil
}

func (api *UpgradeStepsAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (Machine, error) {
	if !canAccess(tag) {
		return nil, apiservererrors.ErrPerm
	}
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	var machine Machine
	var ok bool
	if machine, ok = entity.(Machine); !ok {
		return nil, errors.NotValidf("machine entity")
	}
	return machine, nil
}

func (api *UpgradeStepsAPI) getUnit(canAccess common.AuthFunc, tag names.UnitTag) (Unit, error) {
	if !canAccess(tag) {
		api.logger.Criticalf("getUnit kind=%q, name=%q", tag.Kind(), tag.Id())
		return nil, apiservererrors.ErrPerm
	}
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		api.logger.Criticalf("unable to find entity %q", tag, err)
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// unit.
	var unit Unit
	var ok bool
	if unit, ok = entity.(Unit); !ok {
		return nil, errors.NotValidf("unit entity")
	}
	return unit, nil
}
