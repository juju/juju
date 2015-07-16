// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stateStepsFor121 returns upgrade steps form Juju 1.21 that manipulate state directly.
func stateStepsFor121() []Step {
	return []Step{
		&upgradeStep{
			description: "add environment uuid to state server doc",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvironmentUUIDToStateServerDoc(context.State())
			},
		},
		&upgradeStep{
			description: "set environment owner and server uuid",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.SetOwnerAndServerUUIDForEnvironment(context.State())
			},
		},

		&upgradeStep{
			description: "migrate machine instanceId into instanceData",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateMachineInstanceIdToInstanceData(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all machine docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToMachines(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all instanceData docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToInstanceData(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all containerRef docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToContainerRefs(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all service docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToServices(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all unit docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToUnits(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all reboot docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToReboots(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all relations docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToRelations(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all relationscopes docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToRelationScopes(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all minUnit docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToMinUnits(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all cleanup docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToCleanups(context.State())
			},
		},
		&upgradeStep{
			description: "prepend the environment UUID to the ID of all sequence docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToSequences(context.State())
			},
		},

		&upgradeStep{
			description: "rename the user LastConnection field to LastLogin",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateUserLastConnectionToLastLogin(context.State())
			},
		},
		&upgradeStep{
			description: "add all users in state as environment users",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddStateUsersAsEnvironUsers(context.State())
			},
		},
		&upgradeStep{
			description: "migrate custom image metadata into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return migrateCustomImageMetadata(context.State(), context.AgentConfig())
			},
		},
		&upgradeStep{
			description: "migrate tools into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return migrateToolsStorage(context.State(), context.AgentConfig())
			},
		},
		&upgradeStep{
			description: "migrate individual unit ports to openedPorts collection",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateUnitPortsToOpenedPorts(context.State())
			},
		},
		&upgradeStep{
			description: "create entries in meter status collection for existing units",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.CreateUnitMeterStatus(context.State())
			},
		},
		&upgradeStep{
			description: "migrate machine jobs into ones with JobManageNetworking based on rules",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.MigrateJobManageNetworking(context.State())
			},
		},
	}
}
