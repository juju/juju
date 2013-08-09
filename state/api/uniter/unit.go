// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Unit represents a juju unit as seen by a uniter worker.
type Unit struct {
	tag  string
	life params.Life
	st   *State
}

// Tag returns the unit's tag.
func (u *Unit) Tag() string {
	return u.tag
}

// Life returns the unit's lifecycle value.
func (u *Unit) Life() params.Life {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := u.st.unitLife(u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// SetStatus sets the status of the unit.
func (u *Unit) SetStatus(status params.Status, info string) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.SetEntityStatus{
			{Tag: u.tag, Status: status, Info: info},
		},
	}
	err := u.st.caller.Call("Uniter", "", "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (u *Unit) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Uniter", "", "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the unit.
func (u *Unit) Watch() (*watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag}},
	}
	err := u.st.caller.Call("Uniter", "", "Watch", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(u.st.caller, result)
	return w, nil
}
