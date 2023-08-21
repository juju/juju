// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"context"

	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ControllerConfigGetter defines the methods required to get the controller
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// MeterStatus defines the methods exported by the meter status API facade.
type MeterStatus interface {
	GetMeterStatus(ctx context.Context, args params.Entities) (params.MeterStatusResults, error)
	WatchMeterStatus(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error)
}

// MeterStatusState represents the state of a model required by the MeterStatus.
type MeterStatusState interface {
	ApplyOperation(state.ModelOperation) error
	ControllerConfig() (controller.Config, error)

	// Application returns a application state by name.
	Application(name string) (*state.Application, error)

	// Unit returns a unit by name.
	Unit(id string) (*state.Unit, error)
}

// MeterStatusAPI implements the MeterStatus interface and is the concrete
// implementation of the API endpoint. Additionally, it embeds
// common.UnitStateAPI to allow meter status workers to access their
// controller-backed internal state.
type MeterStatusAPI struct {
	*common.UnitStateAPI

	state      MeterStatusState
	accessUnit common.GetAuthFunc
	resources  facade.Resources
}

// NewMeterStatusAPI creates a new API endpoint for dealing with unit meter status.
func NewMeterStatusAPI(
	controllerConfigGetter ControllerConfigGetter,
	st MeterStatusState,
	resources facade.Resources,
	authorizer facade.Authorizer,
	logger loggo.Logger,
) (*MeterStatusAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	accessUnit := unitcommon.UnitAccessor(authorizer, unitcommon.Backend(st))
	return &MeterStatusAPI{
		state:      st,
		accessUnit: accessUnit,
		resources:  resources,
		UnitStateAPI: common.NewUnitStateAPI(
			controllerConfigGetter,
			unitStateShim{st},
			resources,
			authorizer,
			accessUnit,
			logger,
		),
	}, nil
}

// WatchMeterStatus returns a NotifyWatcher for observing changes
// to each unit's meter status.
func (m *MeterStatusAPI) WatchMeterStatus(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := m.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		var watcherId string
		if canAccess(tag) {
			watcherId, err = m.watchOneUnitMeterStatus(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (m *MeterStatusAPI) watchOneUnitMeterStatus(tag names.UnitTag) (string, error) {
	unit, err := m.state.Unit(tag.Id())
	if err != nil {
		return "", err
	}
	watch := unit.WatchMeterStatus()
	if _, ok := <-watch.Changes(); ok {
		return m.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// GetMeterStatus returns meter status information for each unit.
func (m *MeterStatusAPI) GetMeterStatus(ctx context.Context, args params.Entities) (params.MeterStatusResults, error) {
	result := params.MeterStatusResults{
		Results: make([]params.MeterStatusResult, len(args.Entities)),
	}
	canAccess, err := m.accessUnit()
	if err != nil {
		return params.MeterStatusResults{}, apiservererrors.ErrPerm
	}
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		var status state.MeterStatus
		if canAccess(unitTag) {
			var unit *state.Unit
			unit, err = m.state.Unit(unitTag.Id())
			if err == nil {
				status, err = MeterStatusWrapper(unit.GetMeterStatus)
			}
			result.Results[i].Code = status.Code.String()
			result.Results[i].Info = status.Info
		}
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// unitStateShim adapts the state backend for this facade to make it compatible
// with common.UnitStateAPI.
type unitStateShim struct {
	st MeterStatusState
}

func (s unitStateShim) ApplyOperation(op state.ModelOperation) error {
	return s.st.ApplyOperation(op)
}

func (s unitStateShim) Unit(name string) (common.UnitStateUnit, error) {
	return s.st.Unit(name)
}
