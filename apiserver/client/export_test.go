// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import "github.com/juju/juju/state"

var (
	RemoteParamsForMachine = remoteParamsForMachine
	GetAllUnitNames        = getAllUnitNames
)

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

var (
	StartSerialWaitParallel = startSerialWaitParallel
	GetEnvironment          = &getEnvironment
)

type StateInterface stateInterface

type Patcher interface {
	PatchValue(ptr, value interface{})
}

func PatchState(p Patcher, st StateInterface) {
	p.PatchValue(&getState, func(*state.State) stateInterface {
		return st
	})
}
