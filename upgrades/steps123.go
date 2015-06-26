// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/names"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter"
)

// stateStepsFor123 returns upgrade steps for Juju 1.23 that manipulate state directly.
func stateStepsFor123() []Step {
	return []Step{
		&upgradeStep{
			description: "add default storage pools",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return addDefaultStoragePools(context.State())
			},
		},
		&upgradeStep{
			description: "drop old mongo indexes",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.DropOldIndexesv123(context.State())
			},
		}, &upgradeStep{
			description: "migrate envuuid to env-uuid in envUsersC",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToEnvUsersDoc(context.State())
			},
		},
		&upgradeStep{
			description: "move blocks from environment to state",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return moveBlocksFromEnvironToState(context)
			},
		}, &upgradeStep{
			description: "insert userenvnameC doc for each environment",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddUniqueOwnerEnvNameForEnvirons(context.State())
			},
		}, &upgradeStep{
			description: "add name field to users and lowercase _id field",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddNameFieldLowerCaseIdOfUsers(context.State())
			},
		}, &upgradeStep{
			description: "add life field to IP addresses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddLifeFieldOfIPAddresses(context.State())
			},
		}, &upgradeStep{
			description: "add instance id field to IP addresses",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddInstanceIdFieldOfIPAddresses(context.State())
			},
		}, &upgradeStep{
			description: "lower case _id of envUsers",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.LowerCaseEnvUsersID(context.State())
			},
		}, &upgradeStep{
			description: "add leadership settings documents for all services",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddLeadershipSettingsDocs(context.State())
			},
		},
	}
}

// stepsFor123 returns upgrade steps for Juju 1.23 that only need the API.
func stepsFor123() []Step {
	return []Step{
		&upgradeStep{
			description: "add environment UUID to agent config",
			targets:     []Target{AllMachines},
			run:         addEnvironmentUUIDToAgentConfig,
		},
		&upgradeStep{
			description: "add Stopped field to uniter state",
			targets:     []Target{AllMachines},
			run: func(context Context) error {
				config := context.AgentConfig()
				tag, ok := config.Tag().(names.UnitTag)
				if !ok {
					// not a Unit; skipping
					return nil
				}
				return uniter.AddStoppedFieldToUniterState(tag, config.DataDir())
			},
		},
	}
}
