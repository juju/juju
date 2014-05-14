// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// Machine represents a juju machine as seen by the firewaller worker.
type Machine struct {
	st   *State
	tag  string
	life params.Life
}

// WatchUnits starts a StringsWatcher to watch all units assigned to
// the machine.
func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.st.call("WatchUnits", args, &results)
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
	w := watcher.NewStringsWatcher(m.st.caller, result)
	return w, nil
}

// InstanceId returns the provider specific instance id for this
// machine, or a CodeNotProvisioned error, if not set.
func (m *Machine) InstanceId() (instance.Id, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.st.call("InstanceId", args, &results)
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
