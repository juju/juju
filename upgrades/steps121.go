// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// stateStepsFor121 returns upgrades steps that manipulate State directly for Juju 1.21.
func stateStepsFor121() []StateStep {
	return []StateStep{
		&stateUpgradeStep{
			description: "add environment uuid to state server doc",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvironmentUUIDToStateServerDoc(context.State())
			},
		},
		&stateUpgradeStep{
			description: "set environment owner and server uuid",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.SetOwnerAndServerUUIDForEnvironment(context.State())
			},
		},

		&stateUpgradeStep{
			description: "migrate machine instanceId into instanceData",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.MigrateMachineInstanceIdToInstanceData(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all machine docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToMachines(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all instanceData docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToInstanceData(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all containerRef docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToContainerRefs(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all service docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToServices(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all unit docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToUnits(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all reboot docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToReboots(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all relations docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToRelations(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all relationscopes docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToRelationScopes(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all charm docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToCharms(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all minUnit docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToMinUnits(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all cleanup docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToCleanups(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all sequence docs",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddEnvUUIDToSequences(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all settings docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToSettings(context.State())
			},
		},
		&stateUpgradeStep{
			description: "prepend the environment UUID to the ID of all settingsRefs docs",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return state.AddEnvUUIDToSettingsRefs(context.State())
			},
		},

		&stateUpgradeStep{
			description: "rename the user LastConnection field to LastLogin",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.MigrateUserLastConnectionToLastLogin(context.State())
			},
		},
		&stateUpgradeStep{
			description: "add all users in state as environment users",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.AddStateUsersAsEnvironUsers(context.State())
			},
		},
		&stateUpgradeStep{
			description: "migrate charm archives into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return migrateCharmStorage(context.State(), context.AgentConfig())
			},
		},
		&stateUpgradeStep{
			description: "migrate custom image metadata into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return migrateCustomImageMetadata(context.State(), context.AgentConfig())
			},
		},
		&stateUpgradeStep{
			description: "migrate tools into environment storage",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return migrateToolsStorage(context.State(), context.AgentConfig())
			},
		},
		&stateUpgradeStep{
			description: "migrate individual unit ports to openedPorts collection",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.MigrateUnitPortsToOpenedPorts(context.State())
			},
		},
		&stateUpgradeStep{
			description: "create entries in meter status collection for existing units",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.CreateUnitMeterStatus(context.State())
			},
		},
		&stateUpgradeStep{
			description: "migrate machine jobs into ones with JobManageNetworking based on rules",
			targets:     []Target{DatabaseMaster},
			run: func(context StateContext) error {
				return state.MigrateJobManageNetworking(context.State())
			},
		},
	}
}

// stepsFor121 returns upgrade steps for Juju 1.21.
func stepsFor121() []Step {
	return nil
}
