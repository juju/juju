// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
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
		return "", result.Error
	}
	return instance.Id(result.Result), nil
}

// Life returns the machine's life cycle value.
func (m *Machine) Life() life.Value {
	return m.life
}

// ActiveSubnets returns a list of subnet tags for which the machine has opened
// ports.
func (m *Machine) ActiveSubnets() ([]names.SubnetTag, error) {
	var results params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("GetMachineActiveSubnets", args, &results)
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
	// Convert string tags to names.SubnetTag before returning.
	tags := make([]names.SubnetTag, len(result.Result))
	for i, tag := range result.Result {
		var subnetTag names.SubnetTag
		if tag != "" {
			subnetTag, err = names.ParseSubnetTag(tag)
			if err != nil {
				return nil, err
			}
		}
		tags[i] = subnetTag
	}
	return tags, nil
}

// OpenedPorts returns a map of network.PortRange to unit tag for all opened
// port ranges on the machine for the subnet matching given subnetTag.
//
// TODO(achilleasa): remove from client once we complete the migration to the
// new API.
// DEPRECATED: use OpenedPortRanges instead.
func (m *Machine) OpenedPorts(subnetTag names.SubnetTag) (map[network.PortRange]names.UnitTag, error) {
	var results params.MachinePortsResults
	var subnetTagAsString string
	if subnetTag.Id() != "" {
		subnetTagAsString = subnetTag.String()
	}
	args := params.MachinePortsParams{
		Params: []params.MachinePorts{
			{MachineTag: m.tag.String(), SubnetTag: subnetTagAsString},
		},
	}
	err := m.st.facade.FacadeCall("GetMachinePorts", args, &results)
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
	// Convert string tags to names.UnitTag before returning.
	endResult := make(map[network.PortRange]names.UnitTag)
	for _, ports := range result.Ports {
		unitTag, err := names.ParseUnitTag(ports.UnitTag)
		if err != nil {
			return nil, err
		}
		endResult[ports.PortRange.NetworkPortRange()] = unitTag
	}
	return endResult, nil
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
// the opened port ranges for each unit grouped by endpoint name and subnet
// CIDR respectively.
func (m *Machine) OpenedMachinePortRanges() (byEndpoint, byCIDR map[names.UnitTag]network.GroupedPortRanges, err error) {
	if m.st.BestAPIVersion() < 6 {
		// OpenedMachinePortRanges() was introduced in FirewallerAPIV6.
		return nil, nil, errors.NotImplementedf("OpenedMachinePortRanges() (need V6+)")
	}

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
	} else if groupLen := len(result.Groups); groupLen != 2 {
		return nil, nil, fmt.Errorf("expected two groups for the unit port ranges; got %d", groupLen)
	} else if result.Groups[0].GroupKey != "endpoint" {
		return nil, nil, fmt.Errorf("expected first unit port range group to be grouped by endpoint, got %s", result.Groups[0].GroupKey)
	} else if result.Groups[1].GroupKey != "cidr" {
		return nil, nil, fmt.Errorf("expected second unit port range group to be grouped by subnet CIDR, got %s", result.Groups[1].GroupKey)
	}

	if byEndpoint, err = parseGroupedPortRanges(result.Groups[0].UnitPortRanges); err != nil {
		return nil, nil, errors.Trace(err)
	}
	if byCIDR, err = parseGroupedPortRanges(result.Groups[1].UnitPortRanges); err != nil {
		return nil, nil, errors.Trace(err)
	}

	return byEndpoint, byCIDR, nil
}

func parseGroupedPortRanges(res []params.OpenUnitPortRanges) (map[names.UnitTag]network.GroupedPortRanges, error) {
	byUnit := make(map[names.UnitTag]network.GroupedPortRanges)
	for _, unitPortRanges := range res {
		unitTag, err := names.ParseUnitTag(unitPortRanges.UnitTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		byUnit[unitTag] = make(network.GroupedPortRanges)

		for cidr, portRanges := range unitPortRanges.PortRangeGroups {
			portList := make([]network.PortRange, len(portRanges))
			for i, pr := range portRanges {
				portList[i] = pr.NetworkPortRange()
			}
			byUnit[unitTag][cidr] = portList
		}
	}

	return byUnit, nil
}
