// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewGetCommandForTest returns a GetCommand with the api provided as specified.
func NewGetCommandForTest(api GetEnvironmentAPI) cmd.Command {
	cmd := &getCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewSetCommandForTest returns a SetCommand with the api provided as specified.
func NewSetCommandForTest(api SetModelAPI) cmd.Command {
	cmd := &setCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewUnsetCommandForTest returns an UnsetCommand with the api provided as specified.
func NewUnsetCommandForTest(api UnsetEnvironmentAPI) cmd.Command {
	cmd := &unsetCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewRetryProvisioningCommandForTest returns a RetryProvisioningCommand with the api provided as specified.
func NewRetryProvisioningCommandForTest(api RetryProvisioningAPI) cmd.Command {
	cmd := &retryProvisioningCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

type ShareCommand struct {
	*shareCommand
}

// NewShareCommandForTest returns a ShareCommand with the api provided as specified.
func NewShareCommandForTest(api ShareEnvironmentAPI) (cmd.Command, *ShareCommand) {
	cmd := &shareCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd), &ShareCommand{cmd}
}

type UnshareCommand struct {
	*unshareCommand
}

// NewUnshareCommandForTest returns an unshareCommand with the api provided as specified.
func NewUnshareCommandForTest(api UnshareEnvironmentAPI) (cmd.Command, *UnshareCommand) {
	cmd := &unshareCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd), &UnshareCommand{cmd}
}

// NewUsersCommandForTest returns a UsersCommand with the api provided as specified.
func NewUsersCommandForTest(api UsersAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &usersCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewDestroyCommandForTest returns a DestroyCommand with the api provided as specified.
func NewDestroyCommandForTest(api DestroyEnvironmentAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &destroyCommand{
		api: api,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(
		cmd,
		modelcmd.ModelSkipDefault,
		modelcmd.ModelSkipFlags,
	)
}
