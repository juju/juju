// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

var (
	ParseSettingsCompatible = parseSettingsCompatible
	RemoteParamsForMachine  = remoteParamsForMachine
	GetAllUnitNames         = getAllUnitNames
	StateStorage            = &stateStorage
)

var MachineJobFromParams = machineJobFromParams

// Filtering exports
var (
	MatchPorts  = matchPorts
	MatchSubnet = matchSubnet
)

// Status exports
var (
	ProcessMachines   = processMachines
	MakeMachineStatus = makeMachineStatus
)

//Client exports
var (
	BlockOperation = blockedOperationError
)

type MachineAndContainers machineAndContainers
