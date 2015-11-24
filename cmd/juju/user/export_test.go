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

func NewAddCommand(api AddUserAPI) (cmd.Command, *AddCommand) {
	c := &addCommand{api: api}
	return envcmd.WrapSystem(c), &AddCommand{c}
}

func NewInfoCommand(api UserInfoAPI) cmd.Command {
	return envcmd.WrapSystem(&infoCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		}})
}

func NewCredentialsCommand() (cmd.Command, *CredentialsCommand) {
	c := &credentialsCommand{}
	return envcmd.WrapSystem(c), &CredentialsCommand{c}
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommand(api ChangePasswordAPI, writer EnvironInfoCredsWriter) (cmd.Command, *ChangePasswordCommand) {
	c := &changePasswordCommand{
		api:    api,
		writer: writer,
	}
	return envcmd.WrapSystem(c), &ChangePasswordCommand{c}
}

// NewDisableCommand returns a DisableCommand with the api provided as
// specified.
func NewDisableCommand(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &disableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return envcmd.WrapSystem(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewEnableCommand returns a EnableCommand with the api provided as
// specified.
func NewEnableCommand(api disenableUserAPI) (cmd.Command, *DisenableUserBase) {
	c := &enableCommand{
		disenableUserBase{
			api: api,
		},
	}
	return envcmd.WrapSystem(c), &DisenableUserBase{&c.disenableUserBase}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommand(api UserInfoAPI) cmd.Command {
	c := &listCommand{
		infoCommandBase: infoCommandBase{
			api: api,
		},
	}
	return envcmd.WrapSystem(c)
}
