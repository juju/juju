// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// Machine represents a juju machine as seen by the firewaller worker.
type Machine struct {
	st   *State
	tag  names.MachineTag
	life params.Life
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
	w := watcher.NewStringsWatcher(m.st.facade.RawAPICaller(), result)
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
func (m *Machine) Life() params.Life {
	return m.life
}

// ActiveNetworks returns a list of network tags for which the machine
// has opened ports.
func (m *Machine) ActiveNetworks() ([]names.NetworkTag, error) {
	var results params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.st.facade.FacadeCall("GetMachineActiveNetworks", args, &results)
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
	// Convert string tags to names.NetworkTag before returning.
	tags := make([]names.NetworkTag, len(result.Result))
	for i, tag := range result.Result {
		networkTag, err := names.ParseNetworkTag(tag)
		if err != nil {
			return nil, err
		}
		tags[i] = networkTag
	}
	return tags, nil
}

// OpenedPorts returns a map of network.PortRange to unit tag for all
// opened port ranges on the machine for the given network tag.
func (m *Machine) OpenedPorts(networkTag names.NetworkTag) (map[network.PortRange]names.UnitTag, error) {
	var results params.MachinePortsResults
	args := params.MachinePortsParams{
		Params: []params.MachinePorts{
			{MachineTag: m.tag.String(), NetworkTag: networkTag.String()},
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
