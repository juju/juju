// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var (
	RandomPasswordNotify = &randomPasswordNotify
	ReadPassword         = &readPassword
)

type AddCommand struct {
	*addCommand
}

type ChangePasswordCommand struct {
	*changePasswordCommand
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

func NewShowUserCommandForTest(api UserInfoAPI) cmd.Command {
	return modelcmd.WrapController(&infoCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		}})
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommandForTest(api ChangePasswordAPI, store jujuclient.ClientStore) (cmd.Command, *ChangePasswordCommand) {
	c := &changePasswordCommand{
		api: api,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &ChangePasswordCommand{c}
}

// NewDisableCommand returns a DisableCommand with the api provided as
// specified.
func NewDisableCommandForTest(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &disableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return modelcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewEnableCommand returns a EnableCommand with the api provided as
// specified.
func NewEnableCommandForTest(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &enableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return modelcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommandForTest(api UserInfoAPI) cmd.Command {
	c := &listCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		},
	}
	return modelcmd.WrapController(c)
}
