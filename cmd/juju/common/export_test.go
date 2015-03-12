// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// NewGetConstraintsCommand returns a GetCommand with the api provided as specified.
func NewGetConstraintsCommand(api ConstraintsAPI) *GetConstraintsCommand {
	return &GetConstraintsCommand{
		api: api,
	}
}

// NewGetConstraintsCommand returns a GetCommand with the api provided as specified.
func NewSetConstraintsCommand(api ConstraintsAPI) *SetConstraintsCommand {
	return &SetConstraintsCommand{
		api: api,
	}
}
