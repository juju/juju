// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewGetCommandForTest returns a GetCommand with the api provided as specified.
func NewGetCommandForTest(api GetModelAPI) cmd.Command {
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
func NewUnsetCommandForTest(api UnsetModelAPI) cmd.Command {
	cmd := &unsetCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewGetDefaultsCommandForTest returns a GetDefaultsCommand with the api provided as specified.
func NewGetDefaultsCommandForTest(api modelDefaultsAPI) cmd.Command {
	cmd := &getDefaultsCommand{
		newAPIFunc: func() (modelDefaultsAPI, error) { return api, nil },
	}
	return modelcmd.Wrap(cmd)
}

// NewSetDefaultsCommandForTest returns a SetDefaultsCommand with the api provided as specified.
func NewSetDefaultsCommandForTest(api setModelDefaultsAPI) cmd.Command {
	cmd := &setDefaultsCommand{
		newAPIFunc: func() (setModelDefaultsAPI, error) { return api, nil },
	}
	return modelcmd.Wrap(cmd)
}

// NewUnsetDefaultsCommandForTest returns a UnsetDefaultsCommand with the api provided as specified.
func NewUnsetDefaultsCommandForTest(api unsetModelDefaultsAPI) cmd.Command {
	cmd := &unsetDefaultsCommand{
		newAPIFunc: func() (unsetModelDefaultsAPI, error) { return api, nil },
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

// NewShowCommandForTest returns a ShowCommand with the api provided as specified.
func NewShowCommandForTest(api ShowModelAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showModelCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewDumpCommandForTest returns a DumpCommand with the api provided as specified.
func NewDumpCommandForTest(api DumpModelAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &dumpCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd)
}

// NewDumpDBCommandForTest returns a DumpDBCommand with the api provided as specified.
func NewDumpDBCommandForTest(api DumpDBAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &dumpDBCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd)
}

// NewDestroyCommandForTest returns a DestroyCommand with the api provided as specified.
func NewDestroyCommandForTest(api DestroyModelAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &destroyCommand{
		api: api,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(
		cmd,
		modelcmd.WrapSkipDefaultModel,
		modelcmd.WrapSkipModelFlags,
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
