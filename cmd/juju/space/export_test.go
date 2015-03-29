// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

func NewCreateCommand(api SpaceAPI) *CreateCommand {
	createCmd := &CreateCommand{}
	createCmd.api = api
	return createCmd
}
