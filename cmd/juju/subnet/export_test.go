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

func NewRemoveCommand(api SubnetAPI) *RemoveCommand {
	removeCmd := &RemoveCommand{}
	removeCmd.api = api
	return removeCmd
}

func NewListCommand(api SubnetAPI) *ListCommand {
	listCmd := &ListCommand{}
	listCmd.api = api
	return listCmd
}

func ListFormat(cmd *ListCommand) string {
	return cmd.out.Name()
}
