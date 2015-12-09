// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

var (
	RandomPasswordNotify = &randomPasswordNotify
	ReadPassword         = &readPassword
	ServerFileNotify     = &serverFileNotify
	WriteServerFile      = writeServerFile
)

type AddCommand struct {
	*addCommand
}

type CredentialsCommand struct {
	*credentialsCommand
}

type ChangePasswordCommand struct {
	*changePasswordCommand
}

type DisenableUserBase struct {
	*disenableUserBase
}

func NewAddCommandForTest(api AddUserAPI) (cmd.Command, *AddCommand) {
	c := &addCommand{api: api}
	return envcmd.WrapController(c), &AddCommand{c}
}

func NewShowUserCommandForTest(api UserInfoAPI) cmd.Command {
	return envcmd.WrapController(&infoCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		}})
}

func NewCredentialsCommandForTest() (cmd.Command, *CredentialsCommand) {
	c := &credentialsCommand{}
	return envcmd.WrapController(c), &CredentialsCommand{c}
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommandForTest(api ChangePasswordAPI, writer EnvironInfoCredsWriter) (cmd.Command, *ChangePasswordCommand) {
	c := &changePasswordCommand{
		api:    api,
		writer: writer,
	}
	return envcmd.WrapController(c), &ChangePasswordCommand{c}
}

// NewDisableCommand returns a DisableCommand with the api provided as
// specified.
func NewDisableCommandForTest(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &disableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return envcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewEnableCommand returns a EnableCommand with the api provided as
// specified.
func NewEnableCommandForTest(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &enableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return envcmd.WrapController(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommandForTest(api UserInfoAPI) cmd.Command {
	c := &listCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		},
	}
	return envcmd.WrapController(c)
}
