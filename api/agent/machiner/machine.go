// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machiner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

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
func (m *Machine) Refresh(ctx context.Context) error {
	l, err := m.client.machineLife(ctx, m.tag)
	if err != nil {
		return err
	}
	m.life = l
	return nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(ctx context.Context, status status.Status, info string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: m.tag.String(), Status: status.String(), Info: info, Data: data},
		},
	}
	err := m.client.facade.FacadeCall(ctx, "SetStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetMachineAddresses sets the machine determined addresses of the machine.
func (m *Machine) SetMachineAddresses(ctx context.Context, addresses []network.MachineAddress) error {
	var result params.ErrorResults
	args := params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{
			{Tag: m.Tag().String(), Addresses: params.FromMachineAddresses(addresses...)},
		},
	}
	err := m.client.facade.FacadeCall(ctx, "SetMachineAddresses", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or
// Dying. It does nothing otherwise.
func (m *Machine) EnsureDead(ctx context.Context) error {
	var result params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.client.facade.FacadeCall(ctx, "EnsureDead", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// Watch returns a watcher for observing changes to the machine.
func (m *Machine) Watch(ctx context.Context) (watcher.NotifyWatcher, error) {
	return common.Watch(ctx, m.client.facade, "Watch", m.tag)
}

// IsController returns true if the provided machine is a controller one.
func (m *Machine) IsController(ctx context.Context, machineName string) (bool, error) {
	var results params.IsControllerResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.client.facade.FacadeCall(ctx, "IsController", args, &results)
	if err != nil {
		return false, errors.Annotate(err, "error from FacadeCall")
	}
	if n := len(results.Results); n != 1 {
		return false, errors.Errorf("expected 1 result, got %d", n)
	}
	var errOut error
	if err := results.Results[0].Error; err != nil {
		errOut = err
	}
	return results.Results[0].IsController, errOut
}

// SetObservedNetworkConfig sets the machine network config as observed on the
// machine.
func (m *Machine) SetObservedNetworkConfig(ctx context.Context, netConfig []params.NetworkConfig) error {
	args := params.SetMachineNetworkConfig{
		Tag:    m.Tag().String(),
		Config: netConfig,
	}
	err := m.client.facade.FacadeCall(ctx, "SetObservedNetworkConfig", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RecordAgentStartInformation reports the host name of the machine and updates
// the start time for the agent.
func (m *Machine) RecordAgentStartInformation(ctx context.Context, hostname string) error {
	var result params.ErrorResults
	args := params.RecordAgentStartInformationArgs{
		Args: []params.RecordAgentStartInformationArg{
			{
				Tag:      m.tag.String(),
				Hostname: hostname,
			},
		},
	}
	err := m.client.facade.FacadeCall(ctx, "RecordAgentStartInformation", args, &result)

	if err != nil {
		return err
	}
	return result.OneError()
}
