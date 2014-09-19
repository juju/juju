// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

// Unit represents a juju unit as seen by a firewaller worker.
type Unit struct {
	st   *State
	tag  names.UnitTag
	life params.Life
}

// Name returns the name of the unit.
func (u *Unit) Name() string {
	return u.tag.Id()
}

// Tag returns the unit tag.
func (u *Unit) Tag() names.UnitTag {
	return u.tag
}

// Life returns the unit's life cycle value.
func (u *Unit) Life() params.Life {
	return u.life
}

// Refresh updates the cached local copy of the unit's data.
func (u *Unit) Refresh() error {
	life, err := u.st.life(u.tag)
	if err != nil {
		return err
	}
	u.life = life
	return nil
}

// Watch returns a watcher for observing changes to the unit.
func (u *Unit) Watch() (watcher.NotifyWatcher, error) {
	if u.st.BestAPIVersion() > 0 {
		// Watch() for units is not allowed in FirewallerAPIV1 and
		// later.
		return nil, errors.NotImplementedf("unit.Watch() (in V1+)")
	}
	return common.Watch(u.st.facade, u.tag)
}

// Service returns the service.
func (u *Unit) Service() (*Service, error) {
	serviceTag := names.NewServiceTag(names.UnitService(u.Name()))
	service := &Service{
		st:  u.st,
		tag: serviceTag,
	}
	// Call Refresh() immediately to get the up-to-date
	// life and other needed locally cached fields.
	err := service.Refresh()
	if err != nil {
		return nil, err
	}
	return service, nil
}

// OpenedPorts returns the list of opened ports for this unit.
//
// NOTE: This differs from state.Unit.OpenedPorts() by returning
// an error as well, because it needs to make an API call.
func (u *Unit) OpenedPorts() ([]network.Port, error) {
	if u.st.BestAPIVersion() > 0 {
		// OpenedPorts() is no longer implemented in FirewallerAPIV1
		// and later.
		return nil, errors.NotImplementedf("unit.OpenedPorts() (in V1+)")
	}
	var results params.PortsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	err := u.st.facade.FacadeCall("OpenedPorts", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Ports, nil
}

// AssignedMachine returns the tag of this unit's assigned machine (if
// any), or a CodeNotAssigned error.
func (u *Unit) AssignedMachine() (names.MachineTag, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: u.tag.String()}},
	}
	emptyTag := names.NewMachineTag("")
	err := u.st.facade.FacadeCall("GetAssignedMachine", args, &results)
	if err != nil {
		return emptyTag, err
	}
	if len(results.Results) != 1 {
		return emptyTag, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return emptyTag, result.Error
	}
	return names.ParseMachineTag(result.Result)
}
