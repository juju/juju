// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/clock"
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

var (
	APIOpen          = &apiOpen
	ListModels       = &listModels
	NewAPIConnection = &newAPIConnection
	LoginClientStore = &loginClientStore
)

const NoModelsMessage = noModelsMessage

type AddCommand struct {
	*addCommand
}

type RemoveCommand struct {
	*removeCommand
}

type ChangePasswordCommand struct {
	*changePasswordCommand
}

type LoginCommand struct {
	*loginCommand
}

type LogoutCommand struct {
	*logoutCommand
}

type DisenableUserBase struct {
	*disenableUserBase
}

func NewAddCommandForTest(api AddUserAPI, store jujuclient.ClientStore, modelAPI modelcmd.ModelAPI) (cmd.Command, *AddCommand) {
	c := &addCommand{api: api}
	c.SetClientStore(store)
	c.SetModelAPI(modelAPI)
	return modelcmd.WrapController(c), &AddCommand{c}
}

func NewRemoveCommandForTest(api RemoveUserAPI, store jujuclient.ClientStore) (cmd.Command, *RemoveCommand) {
	c := &removeCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &RemoveCommand{c}
}

func NewShowUserCommandForTest(api UserInfoAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &infoCommand{infoCommandBase: infoCommandBase{
		clock: clock.WallClock,
		api:   api}}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd)
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommandForTest(
	newAPIConnection func(juju.NewAPIConnectionParams) (api.Connection, error),
	api ChangePasswordAPI,
	store jujuclient.ClientStore,
) (cmd.Command, *ChangePasswordCommand) {
	c := &changePasswordCommand{
		newAPIConnection: newAPIConnection,
		api:              api,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &ChangePasswordCommand{c}
}

// NewLogoutCommand returns a LogoutCommand with the api
// and writer provided as specified.
func NewLogoutCommandForTest(store jujuclient.ClientStore) (cmd.Command, *LogoutCommand) {
	c := &logoutCommand{}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &LogoutCommand{c}
}

// NewDisableCommand returns a DisableCommand with the api provided as
// specified.
func NewDisableCommandForTest(api disenableUserAPI, store jujuclient.ClientStore) (cmd.Command, *DisenableUserBase) {
	c := &disableCommand{disenableUserBase{api: api}}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewEnableCommand returns a EnableCommand with the api provided as
// specified.
func NewEnableCommandForTest(api disenableUserAPI, store jujuclient.ClientStore) (cmd.Command, *DisenableUserBase) {
	c := &enableCommand{disenableUserBase{api: api}}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommandForTest(api UserInfoAPI, modelAPI modelUsersAPI, store jujuclient.ClientStore, clock clock.Clock) cmd.Command {
	c := &listCommand{
		infoCommandBase: infoCommandBase{
			clock: clock,
			api:   api,
		},
		modelUserAPI: modelAPI,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

// NewWhoAmICommandForTest returns a whoAMI command with a mock store.
func NewWhoAmICommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &whoAmICommand{store: store}
	return c
}
