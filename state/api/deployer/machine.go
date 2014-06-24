// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"fmt"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// Machine represents a juju machine as seen by the deployer worker.
type Machine struct {
	tag string
	st  *State
}

// WatchUnits starts a StringsWatcher to watch all units deployed to
// the machine, in order to track which ones should be deployed or
// recalled.
func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag}},
	}
	err := m.st.CallFacade("WatchUnits", args, &results)
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
	w := watcher.NewStringsWatcher(m.st.RawAPICaller(), result)
	return w, nil
}
