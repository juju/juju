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

// NewCreateCommand returns a CreateCommand with the api provided as specified.
func NewCreateCommand(api CreateEnvironmentAPI) *CreateCommand {
	return &CreateCommand{
		api: api,
	}
}
