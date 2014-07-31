// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
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

// ActiveNetworks returns the names of the networks the machine has ports opened on.
func (m *Machine) ActiveNetworks() ([]string, error) {
	var result params.StringsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	if err := m.st.facade.FacadeCall("GetMachineActiveNetworks", args, &result); err != nil {
		return nil, err
	}
	if len(result.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return nil, err
	}

	return result.Results[0].Result, nil
}

// GetPorts returns opened port information for the machine.
func (m *Machine) GetPorts(net names.Tag) (map[network.PortRange]string, error) {
	var rawResult params.MachinePortsResults
	args := params.MachinePortsParams{
		Params: []params.MachinePortsParam{{Machine: m.tag.String(), Network: net.String()}},
	}
	if err := m.st.facade.FacadeCall("GetMachinePorts", args, &rawResult); err != nil {
		return nil, err
	}
	if len(rawResult.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(rawResult.Results))
	}
	if err := rawResult.Results[0].Error; err != nil {
		return nil, err
	}
	result := map[network.PortRange]string{}
	for _, portDef := range rawResult.Results[0].Ports {
		result[portDef.Range] = portDef.Unit.Tag
	}
	return result, nil
}
