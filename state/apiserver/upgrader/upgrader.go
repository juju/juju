// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
	st        *state.State
	resources common.ResourceRegistry
}

// New creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(st *state.State, resources common.ResourceRegistry) (*UpgraderAPI, error) {
	return &UpgraderAPI{st: st, resources: resources}, nil
}

const machineTagPrefix = "machine-"

// Start a watcher to track if there is a new version of the API that we want
// to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Agents) (params.EntityWatchResults, error) {
	result := params.EntityWatchResults{
		Results: make([]params.EntityWatchResult, len(args.Tags)),
	}
	for i, _ := range args.Tags {
		var err error
		//if tag[:len(machineTagPrefix)] == machineTagPrefix {
		//    agent, err := u.st.Machine(state.MachineIdFromTag(tag))
		//    // TODO: Auth Check for ownership
		//}
		envWatcher := u.st.WatchEnvironConfig()
		result.Results[i].EntityWatcherId = u.resources.Register(envWatcher)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
