// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

var (
	ParseSettingsCompatible = parseSettingsCompatible
	RemoteParamsForMachine  = remoteParamsForMachine
	GetAllUnitNames         = getAllUnitNames
	NewStateStorage         = &newStateStorage
	NewCharmStore           = &newCharmStore
)

var MachineJobFromParams = machineJobFromParams

// Filtering exports
var (
	MatchPortRanges = matchPortRanges
	MatchSubnet     = matchSubnet
)

// Status exports
var (
	ProcessMachines   = processMachines
	MakeMachineStatus = makeMachineStatus
)

type MachineAndContainers machineAndContainers
