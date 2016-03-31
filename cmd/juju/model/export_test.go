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

type GrantCommand struct {
	*grantCommand
}

type RevokeCommand struct {
	*revokeCommand
}

// NewGrantCommandForTest returns a GrantCommand with the api provided as specified.
func NewGrantCommandForTest(api GrantModelAPI, store jujuclient.ClientStore) (cmd.Command, *GrantCommand) {
	cmd := &grantCommand{
		api: api,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &GrantCommand{cmd}
}

// NewRevokeCommandForTest returns an revokeCommand with the api provided as specified.
func NewRevokeCommandForTest(api RevokeModelAPI, store jujuclient.ClientStore) (cmd.Command, *RevokeCommand) {
	cmd := &revokeCommand{
		api: api,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &RevokeCommand{cmd}
}
