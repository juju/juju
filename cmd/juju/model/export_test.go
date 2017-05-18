// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewConfigCommandForTest returns a configCommand with the api
// provided as specified.
func NewConfigCommandForTest(api configCommandAPI) cmd.Command {
	cmd := &configCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewDefaultsCommandForTest returns a defaultsCommand with the api provided as specified.
func NewDefaultsCommandForTest(apiRoot api.Connection, dAPI defaultsCommandAPI, cAPI cloudAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &defaultsCommand{
		newAPIRoot:     func() (api.Connection, error) { return apiRoot, nil },
		newDefaultsAPI: func(caller base.APICallCloser) defaultsCommandAPI { return dAPI },
		newCloudAPI:    func(caller base.APICallCloser) cloudAPI { return cAPI },
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd)
}

// NewRetryProvisioningCommandForTest returns a RetryProvisioningCommand with the api provided as specified.
func NewRetryProvisioningCommandForTest(api RetryProvisioningAPI) cmd.Command {
	cmd := &retryProvisioningCommand{
		api: api,
	}
	return modelcmd.Wrap(cmd)
}

// NewShowCommandForTest returns a ShowCommand with the api provided as specified.
func NewShowCommandForTest(api ShowModelAPI, refreshFunc func(jujuclient.ClientStore, string) error, store jujuclient.ClientStore) cmd.Command {
	cmd := &showModelCommand{api: api, RefreshModels: refreshFunc}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd, modelcmd.WrapSkipModelFlags)
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
func NewDestroyCommandForTest(
	api DestroyModelAPI,
	configApi ModelConfigAPI,
	refreshFunc func(jujuclient.ClientStore, string) error, store jujuclient.ClientStore,
	sleepFunc func(time.Duration),
) cmd.Command {
	cmd := &destroyCommand{
		api:           api,
		configApi:     configApi,
		RefreshModels: refreshFunc,
		sleepFunc:     sleepFunc,
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
func NewGrantCommandForTest(modelsApi GrantModelAPI, offersAPI GrantOfferAPI, store jujuclient.ClientStore) (cmd.Command, *GrantCommand) {
	cmd := &grantCommand{
		modelsApi: modelsApi,
		offersApi: offersAPI,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &GrantCommand{cmd}
}

// NewRevokeCommandForTest returns an revokeCommand with the api provided as specified.
func NewRevokeCommandForTest(modelsApi RevokeModelAPI, offersAPI RevokeOfferAPI, store jujuclient.ClientStore) (cmd.Command, *RevokeCommand) {
	cmd := &revokeCommand{
		modelsApi: modelsApi,
		offersApi: offersAPI,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &RevokeCommand{cmd}
}

var GetBudgetAPIClient = &getBudgetAPIClient
