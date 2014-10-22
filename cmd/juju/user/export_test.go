// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"github.com/juju/cmd"
)

var (
	ReadPassword = &readPassword
	// add
	GetAddUserAPI  = &getAddUserAPI
	GetShareEnvAPI = &getShareEnvAPI
	// change password
	GetChangePasswordAPI     = &getChangePasswordAPI
	GetEnvironInfoWriter     = &getEnvironInfoWriter
	GetConnectionCredentials = &getConnectionCredentials
	// disable and enable
	GetDisableUserAPI = &getDisableUserAPI

	UserFriendlyDuration = userFriendlyDuration
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
