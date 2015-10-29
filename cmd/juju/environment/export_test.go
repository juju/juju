// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

var (
	NewEnvGetConstraintsCommand = newEnvGetConstraintsCommand
	NewEnvSetConstraintsCommand = newEnvSetConstraintsCommand
)

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommand(api GetEnvironmentAPI) cmd.Command {
	cmd := &getCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewSetCommand returns a SetCommand with the api provided as specified.
func NewSetCommand(api SetEnvironmentAPI) cmd.Command {
	cmd := &setCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewUnsetCommand returns an UnsetCommand with the api provided as specified.
func NewUnsetCommand(api UnsetEnvironmentAPI) cmd.Command {
	cmd := &unsetCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewRetryProvisioningCommand returns a RetryProvisioningCommand with the api provided as specified.
func NewRetryProvisioningCommand(api RetryProvisioningAPI) cmd.Command {
	cmd := &retryProvisioningCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

type ShareCommand struct {
	*shareCommand
}

// NewShareCommand returns a ShareCommand with the api provided as specified.
func NewShareCommand(api ShareEnvironmentAPI) (cmd.Command, *ShareCommand) {
	cmd := &shareCommand{
		api: api,
	}
	return envcmd.Wrap(cmd), &ShareCommand{cmd}
}

type UnshareCommand struct {
	*unshareCommand
}

// NewUnshareCommand returns an unshareCommand with the api provided as specified.
func NewUnshareCommand(api UnshareEnvironmentAPI) (cmd.Command, *UnshareCommand) {
	cmd := &unshareCommand{
		api: api,
	}
	return envcmd.Wrap(cmd), &UnshareCommand{cmd}
}

// NewUsersCommand returns a UsersCommand with the api provided as specified.
func NewUsersCommand(api UsersAPI) cmd.Command {
	cmd := &usersCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewDestroyCommand returns a DestroyCommand with the api provided as specified.
func NewDestroyCommand(api DestroyEnvironmentAPI) cmd.Command {
	cmd := &destroyCommand{
		api: api,
	}
	return envcmd.Wrap(
		cmd,
		envcmd.EnvSkipDefault,
		envcmd.EnvSkipFlags,
	)
}
