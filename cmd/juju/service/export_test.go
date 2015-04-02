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

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommand(api GetServiceAPI) *GetCommand {
	return &GetCommand{
		api: api,
	}
}

// NewAddUnitCommand returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommand(api ServiceAddUnitAPI) *AddUnitCommand {
	return &AddUnitCommand{
		api: api,
	}
}
