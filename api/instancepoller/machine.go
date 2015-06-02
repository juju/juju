// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Machine represents a juju machine as seen by an instancepoller
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life params.Life
}

// Id returns the machine's id.
func (m *Machine) Id() string {
	return m.tag.Id()
}

// Tag returns the machine's tag.
func (m *Machine) Tag() names.MachineTag {
	return m.tag
}

// String returns the machine as a string.
func (m *Machine) String() string {
	return m.Id()
}

// Life returns the machine's lifecycle value.
func (m *Machine) Life() params.Life {
	return m.life
}

// Refresh updates the cached local copy of the machine's data.
func (m *Machine) Refresh() error {
	life, err := common.Life(m.facade, m.tag)
	if err != nil {
		return errors.Trace(err)
	}
	m.life = life
	return nil
}

// Status returns the machine status.
func (m *Machine) Status() (params.StatusResult, error) {
	var results params.StatusResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("Status", args, &results)
	if err != nil {
		return params.StatusResult{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return params.StatusResult{}, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.StatusResult{}, result.Error
	}
	return result, nil
}

// IsManual returns whether the machine is manually provisioned.
func (m *Machine) IsManual() (bool, error) {
	var results params.BoolResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("AreManuallyProvisioned", args, &results)
	if err != nil {
		return false, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return false, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	return result.Result, nil
}

// InstanceId returns the machine's instance id.
func (m *Machine) InstanceId() (instance.Id, error) {
	var results params.StringResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("InstanceId", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return "", err
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return instance.Id(result.Result), nil
}

// InstanceStatus returns the machine's instance status.
func (m *Machine) InstanceStatus() (string, error) {
	var results params.StringResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("InstanceStatus", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return "", err
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// SetInstanceStatus sets the instance status of the machine.
func (m *Machine) SetInstanceStatus(status string) error {
	var result params.ErrorResults
	args := params.SetInstancesStatus{Entities: []params.InstanceStatus{
		{Tag: m.tag.String(), Status: status},
	}}
	err := m.facade.FacadeCall("SetInstanceStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// ProviderAddresses returns all addresses of the machine known to the
// cloud provider.
func (m *Machine) ProviderAddresses() ([]network.Address, error) {
	var results params.MachineAddressesResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("ProviderAddresses", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return nil, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return params.NetworkAddresses(result.Addresses), nil
}

// SetProviderAddresses sets the cached provider addresses for the
// machine.
func (m *Machine) SetProviderAddresses(addrs ...network.Address) error {
	var result params.ErrorResults
	args := params.SetMachinesAddresses{
		MachineAddresses: []params.MachineAddresses{{
			Tag:       m.tag.String(),
			Addresses: params.FromNetworkAddresses(addrs),
		}}}
	err := m.facade.FacadeCall("SetProviderAddresses", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
