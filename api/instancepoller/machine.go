// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
)

// Machine represents a juju machine as seen by an instancepoller
// worker.
type Machine struct {
	facade base.FacadeCaller

	tag  names.MachineTag
	life life.Value
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
func (m *Machine) Life() life.Value {
	return m.life
}

// Refresh updates the cached local copy of the machine's data.
func (m *Machine) Refresh() error {
	life, err := common.OneLife(m.facade, m.tag)
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
func (m *Machine) InstanceStatus() (params.StatusResult, error) {
	var results params.StatusResults
	args := params.Entities{Entities: []params.Entity{
		{Tag: m.tag.String()},
	}}
	err := m.facade.FacadeCall("InstanceStatus", args, &results)
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

// SetInstanceStatus sets the instance status of the machine.
func (m *Machine) SetInstanceStatus(status status.Status, message string, data map[string]interface{}) error {
	var result params.ErrorResults
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: m.tag.String(), Status: status.String(), Info: message, Data: data},
	}}
	err := m.facade.FacadeCall("SetInstanceStatus", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// SetProviderNetworkConfig updates the provider addresses for this machine.
func (m *Machine) SetProviderNetworkConfig(ifList []network.InterfaceInfo) (network.ProviderAddresses, bool, error) {
	var results params.SetProviderNetworkConfigResults
	args := params.SetProviderNetworkConfig{
		Args: []params.ProviderNetworkConfig{{
			Tag:     m.tag.String(),
			Configs: params.NetworkConfigFromInterfaceInfo(ifList),
		}},
	}

	err := m.facade.FacadeCall("SetProviderNetworkConfig", args, &results)
	if err != nil {
		return nil, false, err
	}

	if len(results.Results) != 1 {
		err := errors.Errorf("expected 1 result, got %d", len(results.Results))
		return nil, false, err
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, false, result.Error
	}

	return params.ToProviderAddresses(result.Addresses...), result.Modified, nil
}
