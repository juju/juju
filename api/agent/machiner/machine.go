// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Machine represents a juju machine as seen by a machiner worker.
type Machine struct {
	tag    names.MachineTag
	life   life.Value
	client *Client
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
	l, err := m.client.machineLife(m.tag)
	if err != nil {
		return err
	}
	m.life = l
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
	err := m.client.facade.FacadeCall(context.TODO(), "SetStatus", args, &result)
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
	err := m.client.facade.FacadeCall(context.TODO(), "SetMachineAddresses", args, &result)
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
	err := m.client.facade.FacadeCall(context.TODO(), "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(m.client.facade, "Watch", m.tag)
}

// Jobs returns a list of jobs for the machine.
func (m *Machine) Jobs() (*params.JobsResult, error) {
	var results params.JobsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.Tag().String()}},
	}
	err := m.client.facade.FacadeCall(context.TODO(), "Jobs", args, &results)
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
	err := m.client.facade.FacadeCall(context.TODO(), "SetObservedNetworkConfig", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RecordAgentStartInformation reports the host name of the machine and updates
// the start time for the agent.
func (m *Machine) RecordAgentStartInformation(hostname string) error {
	var result params.ErrorResults
	args := params.RecordAgentStartInformationArgs{
		Args: []params.RecordAgentStartInformationArg{
			{
				Tag:      m.tag.String(),
				Hostname: hostname,
			},
		},
	}
	err := m.client.facade.FacadeCall(context.TODO(), "RecordAgentStartInformation", args, &result)

	if err != nil {
		return err
	}
	return result.OneError()
}
