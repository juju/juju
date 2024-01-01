// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Machine represents a juju machine as seen by the firewaller worker.
type Machine struct {
	st   *Client
	tag  names.MachineTag
	life life.Value
}

// Tag returns the machine tag.
func (m *Machine) Tag() names.MachineTag {
	return m.tag
}

// WatchUnits starts a StringsWatcher to watch all units assigned to
// the machine.
func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("WatchUnits", args, &results)
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
	w := apiwatcher.NewStringsWatcher(m.st.facade.RawAPICaller(), result)
	return w, nil
}

// InstanceId returns the provider specific instance id for this
// machine, or a CodeNotProvisioned error, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("InstanceId", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotProvisioned(result.Error) {
			return "", errors.NotProvisionedf("machine %v", m.tag.Id())
		}
		return "", result.Error
	}
	return instance.Id(result.Result), nil
}

// Life returns the machine's life cycle value.
func (m *Machine) Life() life.Value {
	return m.life
}

// IsManual returns true if the machine was manually provisioned.
func (m *Machine) IsManual() (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("AreManuallyProvisioned", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	return result.Result, nil
}

// OpenedMachinePortRanges queries the open port ranges for all units on this
// machine and returns back two maps where keys are unit names and values are
// open port range groupings by subnet CIDR and endpoint name.
func (m *Machine) OpenedMachinePortRanges() (byUnitAndCIDR map[names.UnitTag]network.GroupedPortRanges, byUnitAndEndpoint map[names.UnitTag]network.GroupedPortRanges, err error) {
	var results params.OpenMachinePortRangesResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	if err = m.st.facade.FacadeCall("OpenedMachinePortRanges", args, &results); err != nil {
		return nil, nil, err
	}
	if len(results.Results) != 1 {
		return nil, nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, nil, result.Error
	}

	byUnitAndCIDR = make(map[names.UnitTag]network.GroupedPortRanges)
	byUnitAndEndpoint = make(map[names.UnitTag]network.GroupedPortRanges)
	for unitTagStr, unitPortRangeList := range result.UnitPortRanges {
		unitTag, err := names.ParseUnitTag(unitTagStr)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		byUnitAndCIDR[unitTag] = make(network.GroupedPortRanges)
		byUnitAndEndpoint[unitTag] = make(network.GroupedPortRanges)

		for _, unitPortRanges := range unitPortRangeList {
			portList := make([]network.PortRange, len(unitPortRanges.PortRanges))
			for i, pr := range unitPortRanges.PortRanges {
				portList[i] = pr.NetworkPortRange()
			}

			byUnitAndEndpoint[unitTag][unitPortRanges.Endpoint] = append(byUnitAndEndpoint[unitTag][unitPortRanges.Endpoint], portList...)
			for _, cidr := range unitPortRanges.SubnetCIDRs {
				byUnitAndCIDR[unitTag][cidr] = append(byUnitAndCIDR[unitTag][cidr], portList...)
			}
		}
	}
	return byUnitAndCIDR, byUnitAndEndpoint, nil
}
