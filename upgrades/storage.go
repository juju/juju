// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage/pool"
)

func addDefaultStoragePools(st *state.State, agentConfig agent.Config) error {
	settings := state.NewStateSettings(st)
	return pool.AddDefaultStoragePools(settings, agentConfig)
}
