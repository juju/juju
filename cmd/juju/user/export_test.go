// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

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

func NewAddCommandForTest(api AddUserAPI, store jujuclient.ClientStore, modelApi modelcmd.ModelAPI) (cmd.Command, *AddCommand) {
	c := &addCommand{api: api}
	c.SetClientStore(store)
	c.SetModelApi(modelApi)
	return modelcmd.WrapController(c), &AddCommand{c}
}

func NewRemoveCommandForTest(api RemoveUserAPI, store jujuclient.ClientStore) (cmd.Command, *RemoveCommand) {
	c := &removeCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &RemoveCommand{c}
}

func NewShowUserCommandForTest(api UserInfoAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &infoCommand{infoCommandBase: infoCommandBase{api: api}}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd)
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommandForTest(api ChangePasswordAPI, store jujuclient.ClientStore) (cmd.Command, *ChangePasswordCommand) {
	c := &changePasswordCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &ChangePasswordCommand{c}
}

// NewLoginCommand returns a LoginCommand with the api
// and writer provided as specified.
func NewLoginCommandForTest(
	newLoginAPI func(juju.NewAPIConnectionParams) (LoginAPI, ConnectionAPI, error),
	store jujuclient.ClientStore,
) (cmd.Command, *LoginCommand) {
	c := &loginCommand{newLoginAPI: newLoginAPI}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &LoginCommand{c}
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
func NewListCommandForTest(api UserInfoAPI, store jujuclient.ClientStore) cmd.Command {
	c := &listCommand{infoCommandBase: infoCommandBase{api: api}}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

// NewWhoAmICommandForTest returns a whoAMI command with a mock store.
func NewWhoAmICommandForTest(store jujuclient.ClientStore) cmd.Command {
	c := &whoAmICommand{store: store}
	return c
}
