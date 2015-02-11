// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

// stateStepsFor123 returns upgrade steps form Juju 1.23 that manipulate state directly.
func stateStepsFor123() []Step {
	var steps []Step
	// TODO(axw) stop checking feature flag once storage has graduated.
	if featureflag.Enabled(feature.Storage) {
		steps = append(steps,
			// TODO - move to api steps once api is available
			&upgradeStep{
				description: "add default storage pools",
				targets:     []Target{DatabaseMaster},
				run: func(context Context) error {
					return addDefaultStoragePools(context.State(), context.AgentConfig())
				},
			},
		)
	}
	steps = append(steps,
		&upgradeStep{
			description: "drop old mongo indexes",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.DropOldIndexesv123(context.State())
			},
		},
	)
	return steps
}

// stepsFor123 returns upgrade steps form Juju 1.23 that only need the API.
func stepsFor123() []Step {
	return []Step{
		&upgradeStep{
			description: "add environment UUID to agent config",
			targets:     []Target{AllMachines},
			run:         addEnvironmentUUIDToAgentConfig,
		},
	}
}
