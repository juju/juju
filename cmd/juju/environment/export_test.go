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

// NewGetCommandForTest returns a GetCommand with the api provided as specified.
func NewGetCommandForTest(api GetEnvironmentAPI) cmd.Command {
	cmd := &getCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewSetCommandForTest returns a SetCommand with the api provided as specified.
func NewSetCommandForTest(api SetEnvironmentAPI) cmd.Command {
	cmd := &setCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewUnsetCommandForTest returns an UnsetCommand with the api provided as specified.
func NewUnsetCommandForTest(api UnsetEnvironmentAPI) cmd.Command {
	cmd := &unsetCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewRetryProvisioningCommandForTest returns a RetryProvisioningCommand with the api provided as specified.
func NewRetryProvisioningCommandForTest(api RetryProvisioningAPI) cmd.Command {
	cmd := &retryProvisioningCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

type ShareCommand struct {
	*shareCommand
}

// NewShareCommandForTest returns a ShareCommand with the api provided as specified.
func NewShareCommandForTest(api ShareEnvironmentAPI) (cmd.Command, *ShareCommand) {
	cmd := &shareCommand{
		api: api,
	}
	return envcmd.Wrap(cmd), &ShareCommand{cmd}
}

type UnshareCommand struct {
	*unshareCommand
}

// NewUnshareCommandForTest returns an unshareCommand with the api provided as specified.
func NewUnshareCommandForTest(api UnshareEnvironmentAPI) (cmd.Command, *UnshareCommand) {
	cmd := &unshareCommand{
		api: api,
	}
	return envcmd.Wrap(cmd), &UnshareCommand{cmd}
}

// NewUsersCommandForTest returns a UsersCommand with the api provided as specified.
func NewUsersCommandForTest(api UsersAPI) cmd.Command {
	cmd := &usersCommand{
		api: api,
	}
	return envcmd.Wrap(cmd)
}

// NewDestroyCommandForTest returns a DestroyCommand with the api provided as specified.
func NewDestroyCommandForTest(api DestroyEnvironmentAPI) cmd.Command {
	cmd := &destroyCommand{
		api: api,
	}
	return envcmd.Wrap(
		cmd,
		envcmd.EnvSkipDefault,
		envcmd.EnvSkipFlags,
	)
}
