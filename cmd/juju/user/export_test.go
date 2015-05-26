// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
)

var (
	ReadPassword = &readPassword
	// disable and enable
	GetDisableUserAPI = &getDisableUserAPI
)

// DisenableCommand is used for testing both Disable and Enable user commands.
type DisenableCommand interface {
	cmd.Command
	Username() string
}

func (c *DisableCommand) Username() string {
	return c.user
}

func (c *EnableCommand) Username() string {
	return c.user
}

var (
	_ DisenableCommand = (*DisableCommand)(nil)
	_ DisenableCommand = (*EnableCommand)(nil)
)

// NewAddCommand returns an AddCommand with the api provided as specified.
func NewAddCommand(api AddUserAPI) *AddCommand {
	return &AddCommand{
		api: api,
	}
}

// NewChangePasswordCommand returns a ChangePasswordCommand with the api
// and writer provided as specified.
func NewChangePasswordCommand(api ChangePasswordAPI, writer EnvironInfoCredsWriter) *ChangePasswordCommand {
	return &ChangePasswordCommand{
		api:    api,
		writer: writer,
	}
}

// NewInfoCommand returns an InfoCommand with the api provided as specified.
func NewInfoCommand(api UserInfoAPI) *InfoCommand {
	return &InfoCommand{
		InfoCommandBase: InfoCommandBase{
			api: api,
		},
	}
}

// NewListCommand returns a ListCommand with the api provided as specified.
func NewListCommand(api UserInfoAPI) *ListCommand {
	return &ListCommand{
		InfoCommandBase: InfoCommandBase{
			api: api,
		},
	}
}
