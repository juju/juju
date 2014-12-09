// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

var (
	ManualProvisioner = &manualProvisioner
)

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommand(api AddMachineAPI) *AddCommand {
	return &AddCommand{
		api: api,
	}
}

// NewRemoveCommand returns an RemoveCommand with the api provided as specified.
func NewRemoveCommand(api RemoveMachineAPI) *RemoveCommand {
	return &RemoveCommand{
		api: api,
	}
}
