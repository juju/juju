// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides the meter status API facade.
package meterstatus

import (
	"gopkg.in/juju/names.v3"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// MeterStatus defines the methods exported by the meter status API facade.
type MeterStatus interface {
	GetMeterStatus(args params.Entities) (params.MeterStatusResults, error)
	WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error)
}

// MeterStatusState represents the state of an model required by the MeterStatus.
//go:generate mockgen -package mocks -destination mocks/meterstatus_mock.go github.com/juju/juju/apiserver/facades/agent/meterstatus MeterStatusState
type MeterStatusState interface {

	// Application returns a application state by name.
	Application(name string) (*state.Application, error)

	// Unit returns a unit by name.
	Unit(id string) (*state.Unit, error)
}

// MeterStatusAPI implements the MeterStatus interface and is the concrete implementation
// of the API endpoint.
type MeterStatusAPI struct {
	state      MeterStatusState
	accessUnit common.GetAuthFunc
	resources  facade.Resources
}

// NewMeterStatusFacade provides the signature required for facade registration.
func NewMeterStatusFacade(ctx facade.Context) (*MeterStatusAPI, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewMeterStatusAPI(ctx.State(), resources, authorizer)
}

// NewMeterStatusAPI creates a new API endpoint for dealing with unit meter status.
func NewMeterStatusAPI(
	st MeterStatusState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MeterStatusAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}
	return &MeterStatusAPI{
		state: st,
		accessUnit: func() (common.AuthFunc, error) {
			switch tag := authorizer.GetAuthTag().(type) {
			case names.ApplicationTag:
				// If called by an application agent, any of the units
				// belonging to that application can be accessed.
				app, err := st.Application(tag.Name)
				if err != nil {
					return nil, errors.Trace(err)
				}
				allUnits, err := app.AllUnits()
				if err != nil {
					return nil, errors.Trace(err)
				}
				return func(tag names.Tag) bool {
					for _, u := range allUnits {
						if u.Tag() == tag {
							return true
						}
					}
					return false
				}, nil
			case names.UnitTag:
				return func(tag names.Tag) bool {
					return authorizer.AuthOwner(tag)
				}, nil
			default:
				return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
			}
		},
		resources: resources,
	}, nil
}

// WatchMeterStatus returns a NotifyWatcher for observing changes
// to each unit's meter status.
func (m *MeterStatusAPI) WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error) {
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
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		var watcherId string
		if canAccess(tag) {
			watcherId, err = m.watchOneUnitMeterStatus(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
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
func (m *MeterStatusAPI) GetMeterStatus(args params.Entities) (params.MeterStatusResults, error) {
	result := params.MeterStatusResults{
		Results: make([]params.MeterStatusResult, len(args.Entities)),
	}
	canAccess, err := m.accessUnit()
	if err != nil {
		return params.MeterStatusResults{}, common.ErrPerm
	}
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
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
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
