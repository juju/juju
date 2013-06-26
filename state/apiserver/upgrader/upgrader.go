// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
	st *state.State
}

// New creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(st *state.State) (*UpgraderAPI, error) {
	return &UpgraderAPI{st: st}, nil
}

// Start a watcher to track if there is a new version of the API that we want
// to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Agents) (params.EntityWatchResults, error) {
	result := params.EntityWatchResults{
		Results: make([]params.EntityWatchResults, len(args.Tags)),
	}
	return result, nil
}
