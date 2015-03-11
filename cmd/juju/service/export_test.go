// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

// NewSetCommand returns a SetCommand with the api provided as specified.
func NewSetCommand(api SetServiceAPI) *SetCommand {
	return &SetCommand{
		api: api,
	}
}

// NewUnsetCommand returns an UnsetCommand with the api provided as specified.
func NewUnsetCommand(api UnsetServiceAPI) *UnsetCommand {
	return &UnsetCommand{
		api: api,
	}
}
