// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/loggo"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/unitcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.meterstatus")

// MeterStatus defines the methods exported by the meter status API facade.
type MeterStatus interface {
	GetMeterStatus(args params.Entities) (params.MeterStatusResults, error)
	WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error)
}

// MeterStatusState represents the state of a model required by the MeterStatus.
//
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/meterstatus_mock.go github.com/juju/juju/apiserver/facades/agent/meterstatus MeterStatusState
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

	resources facade.Resources
}

// NewMeterStatusAPI creates a new API endpoint for dealing with unit meter status.
func NewMeterStatusAPI(
	st MeterStatusState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*MeterStatusAPI, error) {
	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	accessUnit := unitcommon.UnitAccessor(authorizer, unitcommon.Backend(st))
	return &MeterStatusAPI{
		resources: resources,
		UnitStateAPI: common.NewUnitStateAPI(
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
// This is a noop as of 3.6.10 because meter status functionality is removed.
func (m *MeterStatusAPI) WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i := range args.Entities {
		// Create a simple notify watcher that only sends one initial event.
		w := newEmptyNotifyWatcher()
		watcherID := m.resources.Register(w)

		result.Results[i] = params.NotifyWatchResult{
			NotifyWatcherId: watcherID,
		}
	}

	return result, nil
}

// GetMeterStatus returns meter status information for each unit.
// This is a noop as of 3.6.10 because meter status functionality is removed.
func (m *MeterStatusAPI) GetMeterStatus(args params.Entities) (params.MeterStatusResults, error) {
	return params.MeterStatusResults{
		Results: make([]params.MeterStatusResult, len(args.Entities)),
	}, nil
}

// newEmptyNotifyWatcher returns starts and returns a new empty notify watcher,
// with only the initial event.
func newEmptyNotifyWatcher() *emptyNotifyWatcher {
	changes := make(chan struct{})

	w := &emptyNotifyWatcher{
		changes: changes,
	}
	w.tomb.Go(func() error {
		changes <- struct{}{}
		defer close(changes)
		return w.loop()
	})

	return w
}

// emptyNotifyWatcher implements watcher.NotifyWatcher.
type emptyNotifyWatcher struct {
	changes chan struct{}
	tomb    tomb.Tomb
}

// Changes returns the event channel for the empty notify watcher.
func (w *emptyNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *emptyNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Stop asks the watcher to stop and waits.
func (w *emptyNotifyWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *emptyNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

// Err returns any error encountered while the watcher
// has been running.
func (w *emptyNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *emptyNotifyWatcher) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// unitStateShim adapts the state backend for this facade to make it compatible
// with common.UnitStateAPI.
type unitStateShim struct {
	st MeterStatusState
}

func (s unitStateShim) ApplyOperation(op state.ModelOperation) error {
	return s.st.ApplyOperation(op)
}

func (s unitStateShim) ControllerConfig() (controller.Config, error) {
	return s.st.ControllerConfig()
}

func (s unitStateShim) Unit(name string) (common.UnitStateUnit, error) {
	return s.st.Unit(name)
}
