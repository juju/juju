// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

// State provides access to an reboot worker's view of the state.
// NOTE: This is defined as an interface due to PPC64 bug #1533469 -
// if it were a type build errors happen (due to a linker bug).
type State interface {
	// WatchForRebootEvent returns a watcher.NotifyWatcher that
	// reacts to reboot flag changes.
	WatchForRebootEvent() (watcher.NotifyWatcher, error)

	// RequestReboot sets the reboot flag for the calling machine.
	RequestReboot() error

	// ClearReboot clears the reboot flag for the calling machine.
	ClearReboot() error

	// GetRebootAction returns the reboot action for the calling machine.
	GetRebootAction() (params.RebootAction, error)
}

var _ State = (*state)(nil)

// state implements State.
type state struct {
	machineTag names.Tag
	facade     base.FacadeCaller
}

// NewState returns a version of the state that provides functionality
// required by the reboot worker.
func NewState(caller base.APICaller, machineTag names.MachineTag) State {
	return &state{
		facade:     base.NewFacadeCaller(caller, "Reboot"),
		machineTag: machineTag,
	}
}

// WatchForRebootEvent implements State.WatchForRebootEvent
func (st *state) WatchForRebootEvent() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult

	if err := st.facade.FacadeCall("WatchForRebootEvent", nil, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// RequestReboot implements State.RequestReboot
func (st *state) RequestReboot() error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: st.machineTag.String()}},
	}

	err := st.facade.FacadeCall("RequestReboot", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return errors.Trace(results.Results[0].Error)
	}
	return nil
}

// ClearReboot implements State.ClearReboot
func (st *state) ClearReboot() error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: st.machineTag.String()}},
	}

	err := st.facade.FacadeCall("ClearReboot", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return errors.Trace(results.Results[0].Error)
	}

	return nil
}

// GetRebootAction implements State.GetRebootAction
func (st *state) GetRebootAction() (params.RebootAction, error) {
	var results params.RebootActionResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: st.machineTag.String()}},
	}

	err := st.facade.FacadeCall("GetRebootAction", args, &results)
	if err != nil {
		return params.ShouldDoNothing, err
	}
	if len(results.Results) != 1 {
		return params.ShouldDoNothing, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return params.ShouldDoNothing, errors.Trace(results.Results[0].Error)
	}

	return results.Results[0].Result, nil
}
