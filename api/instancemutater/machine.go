// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

// Machine represents a juju machine as seen by an instancemutater
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life params.Life
}

// CharmProfiles returns the CharmProfiles for the machine
func (m *Machine) CharmProfiles() ([]string, error) {
	var result params.StringsResult
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.facade.FacadeCall("CharmProfiles", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Result, nil
}

// SetUpgradeCharmProfileComplete allows setting the charm profile complete
// status message for a given unit.
func (m *Machine) SetUpgradeCharmProfileComplete(unitName string, message string) error {
	var results params.ErrorResults
	args := params.SetProfileUpgradeCompleteArgs{
		Args: []params.SetProfileUpgradeCompleteArg{
			{
				Entity:   params.Entity{Tag: m.tag.String()},
				UnitName: unitName,
				Message:  message,
			},
		},
	}
	err := m.facade.FacadeCall("SetUpgradeCharmProfileComplete", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// Tag returns the current machine tag
func (m *Machine) Tag() names.MachineTag {
	return m.tag
}

// WatchUnits returns a watcher.StringsWatcher for watching units of a given
// machine.
func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.facade.FacadeCall("WatchUnits", args, &results)
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
	w := apiwatcher.NewStringsWatcher(m.facade.RawAPICaller(), result)
	return w, nil
}
