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

//go:generate mockgen -package mocks -destination mocks/machinemutater_mock.go github.com/juju/juju/api/instancemutater MutaterMachine
type MutaterMachine interface {
	// CharmProfilingInfo returns info to update lxd profiles on the machine
	// based on the given unit names.
	CharmProfilingInfo([]string) (ProfileInfo, error)

	// SetCharmProfiles records the given slice of charm profile names.
	SetCharmProfiles([]string) error

	// SetUpgradeCharmProfileComplete records the result of updating
	// the machine's charm profile(s), for the given unit.
	SetUpgradeCharmProfileComplete(unitName string, message string) error

	// Tag returns the current machine tag
	Tag() names.MachineTag

	// RemoveUpgradeCharmProfileData completely removes the instance charm
	// profile data for a machine and the given unit, even if the machine
	// is dead.
	RemoveUpgradeCharmProfileData(string) error

	// WatchUnits returns a watcher.StringsWatcher for watching units of a given
	// machine.
	WatchUnits() (watcher.StringsWatcher, error)
}

// Machine represents a juju machine as seen by an instancemutater
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life params.Life
}

// SetCharmProfiles implements MutaterMachine.SetCharmProfiles.
func (m *Machine) SetCharmProfiles([]string) error {
	return nil
}

// SetUpgradeCharmProfileComplete implements MutaterMachine.SetUpgradeCharmProfileComplete.
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

// Tag implements MutaterMachine.Tag.
func (m *Machine) Tag() names.MachineTag {
	return m.tag
}

// WatchUnits implements MutaterMachine.WatchUnits.
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

type ProfileInfo struct {
	Changes         bool
	ProfileChanges  []ProfileChanges
	CurrentProfiles []string
}

type ProfileChanges struct {
	OldProfileName string
	NewProfileName string
	Profile        *CharmLXDProfile
	Subordinate    bool
}

type CharmLXDProfile struct {
	Config      map[string]string
	Description string
	Devices     map[string]map[string]string
}

// CharmProfilingInfo implements MutaterMachine.CharmProfilingInfo.
func (m *Machine) CharmProfilingInfo(unitNames []string) (*ProfileInfo, error) {
	var result params.CharmProfilingInfoResult
	args := params.CharmProfilingInfoArg{
		Entity:    params.Entity{Tag: m.tag.String()},
		UnitNames: unitNames,
	}
	err := m.facade.FacadeCall("CharmProfilingInfo", args, &result)
	if err != nil {
		return nil, err
	}
	returnResult := &ProfileInfo{
		Changes:         result.Changes,
		CurrentProfiles: result.CurrentProfiles,
	}
	if !result.Changes {
		return returnResult, nil
	}
	profileChanges := make([]ProfileChanges, len(result.ProfileChanges))
	for i, change := range result.ProfileChanges {
		profileChanges[i].NewProfileName = change.NewProfileName
		profileChanges[i].OldProfileName = change.OldProfileName
		profileChanges[i].Subordinate = change.Subordinate
		if change.Profile != nil {
			profileChanges[i].Profile = &CharmLXDProfile{
				Config:      change.Profile.Config,
				Description: change.Profile.Description,
				Devices:     change.Profile.Devices,
			}
		}
		if change.Error != nil {
			return nil, change.Error
		}
	}
	returnResult.ProfileChanges = profileChanges
	return returnResult, nil
}
