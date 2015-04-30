// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import "github.com/juju/juju/storage"

var (
	ManualProvisioner = &manualProvisioner
)

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommand(api AddMachineAPI, mmApi MachineManagerAPI) *AddCommand {
	return &AddCommand{
		api:               api,
		machineManagerAPI: mmApi,
	}
}

// NewRemoveCommand returns an RemoveCommand with the api provided as specified.
func NewRemoveCommand(api RemoveMachineAPI) *RemoveCommand {
	return &RemoveCommand{
		api: api,
	}
}

func NewDisksFlag(disks *[]storage.Constraints) *disksFlag {
	return &disksFlag{disks}
}
