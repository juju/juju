// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

func NewCreateCommand(api SubnetAPI) *CreateCommand {
	createCmd := &CreateCommand{}
	createCmd.api = api
	return createCmd
}

func NewAddCommand(api SubnetAPI) *AddCommand {
	addCmd := &AddCommand{}
	addCmd.api = api
	return addCmd
}
