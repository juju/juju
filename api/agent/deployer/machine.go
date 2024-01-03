// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	"fmt"

	"github.com/juju/names/v5"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Machine represents a juju machine as seen by the deployer worker.
type Machine struct {
	tag    names.MachineTag
	client *Client
}

// WatchUnits starts a StringsWatcher to watch all units deployed to
// the machine, in order to track which ones should be deployed or
// recalled.
func (m *Machine) WatchUnits() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.tag.String()}},
	}
	err := m.client.facade.FacadeCall(context.TODO(), "WatchUnits", args, &results)
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
	w := apiwatcher.NewStringsWatcher(m.client.facade.RawAPICaller(), result)
	return w, nil
}
