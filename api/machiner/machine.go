// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	tag  names.MachineTag
	life life.Value
	st   *State
}

// Tag returns the machine's tag.
func (m *Machine) Tag() names.Tag {
	return m.tag
}

// Life returns the machine's lifecycle value.
func (m *Machine) Life() life.Value {
	return m.life
}

// Refresh updates the cached local copy of the machine's data.
func (m *Machine) Refresh() error {
	life, err := m.st.machineLife(m.tag)
	if err != nil {
		return err
	}
	m.life = life
	return nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.st.facade.FacadeCall("SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetMachineAddresses sets the machine determined addresses of the machine.
func (m *Machine) SetMachineAddresses(addresses []network.MachineAddress) error {
	var result params.ErrorResults
	args := params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: m.Tag().String(), Addresses: params.FromMachineAddresses(addresses...)},
		},
	}
	err := m.st.facade.FacadeCall("SetMachineAddresses", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (m *Machine) EnsureDead() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(m.st.facade, "Watch", m.tag)
}

// Jobs returns a list of jobs for the machine.
func (m *Machine) Jobs() (*params.JobsResult, error) {
	var results params.JobsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.Tag().String()}},
	}
	err := m.st.facade.FacadeCall("Jobs", args, &results)
	if err != nil {
		return nil, errors.Annotate(err, "error from FacadeCall")
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return &result, nil
}

// SetObservedNetworkConfig sets the machine network config as observed on the
// machine.
func (m *Machine) SetObservedNetworkConfig(netConfig []params.NetworkConfig) error {
	args := params.SetMachineNetworkConfig{
		Tag:    m.Tag().String(),
		Config: netConfig,
	}
	err := m.st.facade.FacadeCall("SetObservedNetworkConfig", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SetProviderNetworkConfig sets the machine network config as seen by the
// provider.
func (m *Machine) SetProviderNetworkConfig() error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("SetProviderNetworkConfig", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// RecordAgentStartTime updates the start time for the agent running on the
// machine.
func (m *Machine) RecordAgentStartTime() error {
	// Ignore if connecting to an older, not upgraded controller
	if m.st.facade.BestAPIVersion() < 2 {
		return nil
	}

	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("RecordAgentStartTime", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
