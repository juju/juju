// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides the meter status API facade.
package meterstatus

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var (
	logger = loggo.GetLogger("juju.apiserver.meterstatus")
)

func init() {
	common.RegisterStandardFacade("MeterStatus", 1, NewMeterStatusAPI)
}

// MeterStatus defines the methods exported by the meter status API facade.
type MeterStatus interface {
	GetMeterStatus(args params.Entities) (params.MeterStatusResults, error)
	WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error)
}

// MeterStatusAPI implements the MeterStatus interface and is the concrete implementation
// of the API endpoint.
type MeterStatusAPI struct {
	state      *state.State
	accessUnit common.GetAuthFunc
	resources  *common.Resources
}

var _ MeterStatus = (*MeterStatusAPI)(nil)

// NewMeterStatusAPI creates a new API endpoint for dealing with unit meter status.
func NewMeterStatusAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*MeterStatusAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &MeterStatusAPI{
		state: st,
		accessUnit: func() (common.AuthFunc, error) {
			return authorizer.AuthOwner, nil
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
		watcherId := ""
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
