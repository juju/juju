// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// MutatorMachine represents the machine methods required for the instancemutater.
type MutaterMachine interface {

	// InstanceId returns the provider specific instance id for this machine
	InstanceId(context.Context) (string, error)

	// CharmProfilingInfo returns info to update lxd profiles on the machine
	CharmProfilingInfo(context.Context) (*UnitProfileInfo, error)

	// ContainerType returns the container type for this machine.
	ContainerType(context.Context) (instance.ContainerType, error)

	// SetCharmProfiles records the given slice of charm profile names.
	SetCharmProfiles(context.Context, []string) error

	// Tag returns the current machine tag
	Tag() names.MachineTag

	// Life returns the machine's lifecycle value.
	Life() life.Value

	// Refresh updates the cached local copy of the machine's data.
	Refresh(context.Context) error

	// WatchUnits returns a watcher.StringsWatcher for watching units of a given
	// machine.
	WatchUnits(context.Context) (watcher.StringsWatcher, error)

	// WatchLXDProfileVerificationNeeded returns a NotifyWatcher, notifies when the
	// following changes happen:
	//  - application charm URL changes and there is a lxd profile
	//  - unit is add or removed and there is a lxd profile
	WatchLXDProfileVerificationNeeded(context.Context) (watcher.NotifyWatcher, error)

	// WatchContainers returns a watcher.StringsWatcher for watching
	// containers of a given machine.
	WatchContainers(ctx context.Context) (watcher.StringsWatcher, error)

	// SetModificationStatus sets the provider specific modification status
	// for a machine. Allowing the propagation of status messages to the
	// operator.
	SetModificationStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error
}

// Machine represents a juju machine as seen by an instancemutater
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life life.Value
}

// ContainerType implements MutaterMachine.ContainerType.
func (m *Machine) ContainerType(ctx context.Context) (instance.ContainerType, error) {
	var result params.ContainerTypeResult
	args := params.Entity{Tag: m.tag.String()}
	err := m.facade.FacadeCall(ctx, "ContainerType", args, &result)
	if err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Type, nil
}

// InstanceId implements MutaterMachine.InstanceId.
func (m *Machine) InstanceId(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.facade.FacadeCall(ctx, "InstanceId", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// SetCharmProfiles implements MutaterMachine.SetCharmProfiles.
func (m *Machine) SetCharmProfiles(ctx context.Context, profiles []string) error {
	var results params.ErrorResults
	args := params.SetProfileArgs{
		Args: []params.SetProfileArg{
			{
				Entity:   params.Entity{Tag: m.tag.String()},
				Profiles: profiles,
			},
		},
	}
	err := m.facade.FacadeCall(ctx, "SetCharmProfiles", args, &results)
	if err != nil {
		return err
	}
	if len(results.Results) != 1 {
		return fmt.Errorf("expected 1 result, got %d", len(results.Results))
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

// Life implements MutaterMachine.Life.
func (m *Machine) Life() life.Value {
	return m.life
}

// Refresh implements MutaterMachine.Refresh.
func (m *Machine) Refresh(ctx context.Context) error {
	life, err := common.OneLife(ctx, m.facade, m.tag)
	if err != nil {
		return errors.Trace(err)
	}
	m.life = life
	return nil
}

// WatchUnits implements MutaterMachine.WatchUnits.
func (m *Machine) WatchUnits(ctx context.Context) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.facade.FacadeCall(ctx, "WatchUnits", args, &results)
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

// WatchLXDProfileVerificationNeeded implements MutaterMachine.WatchLXDProfileVerificationNeeded.
func (m *Machine) WatchLXDProfileVerificationNeeded(ctx context.Context) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.facade.FacadeCall(ctx, "WatchLXDProfileVerificationNeeded", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotSupported(result.Error) {
			return nil, errors.NotSupportedf("watching LXD profiles on machine %q", m.tag.Id())
		}
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(m.facade.RawAPICaller(), result), nil
}

// WatchContainers returns a StringsWatcher reporting changes to containers.
func (m *Machine) WatchContainers(ctx context.Context) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	arg := params.Entity{Tag: m.tag.String()}
	err := m.facade.FacadeCall(ctx, "WatchContainers", arg, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewStringsWatcher(m.facade.RawAPICaller(), result), nil
}

// UnitProfileInfo is data required by the instancemutater to determine what
// any changes are required to a machine's lxd profiles.
type UnitProfileInfo struct {
	ModelName       string
	InstanceId      instance.Id
	ProfileChanges  []UnitProfileChanges
	CurrentProfiles []string
}

// UnitProfileChanges provides the application name, revision and profile of
// the current charm referenced by this application.
type UnitProfileChanges struct {
	ApplicationName string
	Revision        int
	Profile         lxdprofile.Profile
}

// CharmProfilingInfo implements MutaterMachine.CharmProfilingInfo.
func (m *Machine) CharmProfilingInfo(ctx context.Context) (*UnitProfileInfo, error) {
	var result params.CharmProfilingInfoResult
	args := params.Entity{Tag: m.tag.String()}
	err := m.facade.FacadeCall(ctx, "CharmProfilingInfo", args, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	returnResult := &UnitProfileInfo{
		InstanceId:      result.InstanceId,
		ModelName:       result.ModelName,
		CurrentProfiles: result.CurrentProfiles,
	}
	profileChanges := make([]UnitProfileChanges, len(result.ProfileChanges))
	for i, change := range result.ProfileChanges {
		var profile lxdprofile.Profile
		if change.Profile != nil {
			profile = lxdprofile.Profile{
				Config:      change.Profile.Config,
				Description: change.Profile.Description,
				Devices:     change.Profile.Devices,
			}
		}
		profileChanges[i] = UnitProfileChanges{
			ApplicationName: change.ApplicationName,
			Revision:        change.Revision,
			Profile:         profile,
		}
		if change.Error != nil {
			return nil, change.Error
		}
	}
	returnResult.ProfileChanges = profileChanges
	return returnResult, nil
}

// SetModificationStatus implements MutaterMachine.SetModificationStatus.
func (m *Machine) SetModificationStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.facade.FacadeCall(ctx, "SetModificationStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
