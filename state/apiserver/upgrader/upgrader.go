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
	st         *state.State
	resources  common.ResourceRegistry
	authorizer common.Authorizer
}

// New creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(
	st *state.State,
	resources common.ResourceRegistry,
	authorizer common.Authorizer,
) (*UpgraderAPI, error) {
	// TODO: Unit agents are also allowed to use this API
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	return &UpgraderAPI{st: st, resources: resources, authorizer: authorizer}, nil
}

const machineTagPrefix = "machine-"

// Start a watcher to track if there is a new version of the API that we want
// to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Agents) (params.EntityWatchResults, error) {
	result := params.EntityWatchResults{
		Results: make([]params.EntityWatchResult, len(args.Tags)),
	}
	for i, tag := range args.Tags {
		var err error
		if !u.authorizer.AuthOwner(tag) {
			err = common.ErrPerm
		} else {
			envWatcher := u.st.WatchEnvironConfig()
			result.Results[i].EntityWatcherId = u.resources.Register(envWatcher)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Find the Tools necessary for a given agent
