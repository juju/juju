// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommand(api GetEnvironmentAPI) *GetCommand {
	return &GetCommand{
		api: api,
	}
}

// NewSetCommand returns a SetCommand with the api provided as specified.
func NewSetCommand(api SetEnvironmentAPI) *SetCommand {
	return &SetCommand{
		api: api,
	}
}

// NewUnsetCommand returns an UnsetCommand with the api provided as specified.
func NewUnsetCommand(api UnsetEnvironmentAPI) *UnsetCommand {
	return &UnsetCommand{
		api: api,
	}
}

// NewRetryProvisioningCommand returns a RetryProvisioningCommand with the api provided as specified.
func NewRetryProvisioningCommand(api RetryProvisioningAPI) *RetryProvisioningCommand {
	return &RetryProvisioningCommand{
		api: api,
	}
}

// NewShareCommand returns a ShareCommand with the api provided as specified.
func NewShareCommand(api ShareEnvironmentAPI) *ShareCommand {
	return &ShareCommand{
		api: api,
	}
}

// NewUnshareCommand returns an unshareCommand with the api provided as specified.
func NewUnshareCommand(api UnshareEnvironmentAPI) *UnshareCommand {
	return &UnshareCommand{
		api: api,
	}
}

// NewUsersCommand returns a UsersCommand with the api provided as specified.
func NewUsersCommand(api UsersAPI) *UsersCommand {
	return &UsersCommand{
		api: api,
	}
}

// NewDestroyCommand returns a DestroyCommand with the api provided as specified.
func NewDestroyCommand(api DestroyEnvironmentAPI) *DestroyCommand {
	return &DestroyCommand{
		api: api,
	}
}
