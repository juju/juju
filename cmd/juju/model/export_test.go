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
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

// NewConfigCommandForTest returns a configCommand with the api
// provided as specified.
func NewConfigCommandForTest(api configCommandAPI) cmd.Command {
	cmd := &configCommand{
		api: api,
	}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
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
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}

// NewShowCommandForTest returns a ShowCommand with the api provided as specified.
func NewShowCommandForTest(api ShowModelAPI, refreshFunc func(jujuclient.ClientStore, string) error, store jujuclient.ClientStore) cmd.Command {
	cmd := &showModelCommand{api: api}
	cmd.SetClientStore(store)
	cmd.SetModelRefresh(refreshFunc)
	return modelcmd.Wrap(cmd, modelcmd.WrapSkipModelFlags)
}

// NewDumpCommandForTest returns a DumpCommand with the api provided as specified.
func NewDumpCommandForTest(api DumpModelAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &dumpCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewDumpDBCommandForTest returns a DumpDBCommand with the api provided as specified.
func NewDumpDBCommandForTest(api DumpDBAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &dumpDBCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewExportBundleCommandForTest returns a ExportBundleCommand with the api provided as specified.
func NewExportBundleCommandForTest(api ExportBundleAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &exportBundleCommand{newAPIFunc: func() (ExportBundleAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewDestroyCommandForTest returns a DestroyCommand with the api provided as specified.
func NewDestroyCommandForTest(
	api DestroyModelAPI,
	configAPI ModelConfigAPI,
	storageAPI StorageAPI,
	refreshFunc func(jujuclient.ClientStore, string) error, store jujuclient.ClientStore,
	sleepFunc func(time.Duration),
) cmd.Command {
	cmd := &destroyCommand{
		api:        api,
		configAPI:  configAPI,
		storageAPI: storageAPI,
		sleepFunc:  sleepFunc,
	}
	cmd.SetClientStore(store)
	cmd.SetModelRefresh(refreshFunc)
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

type GrantCloudCommand struct {
	*grantCloudCommand
}

type RevokeCloudCommand struct {
	*revokeCloudCommand
}

// NewGrantCloudCommandForTest returns a grantCloudCommand with the api provided as specified.
func NewGrantCloudCommandForTest(cloudsApi GrantCloudAPI, store jujuclient.ClientStore) (cmd.Command, *GrantCloudCommand) {
	cmd := &grantCloudCommand{
		cloudsApi: cloudsApi,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &GrantCloudCommand{cmd}
}

// NewRevokeCloudCommandForTest returns a revokeCloudCommand with the api provided as specified.
func NewRevokeCloudCommandForTest(cloudsApi RevokeCloudAPI, store jujuclient.ClientStore) (cmd.Command, *RevokeCloudCommand) {
	cmd := &revokeCloudCommand{
		cloudsApi: cloudsApi,
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(cmd), &RevokeCloudCommand{cmd}
}

func NewModelSetConstraintsCommandForTest() cmd.Command {
	cmd := &modelSetConstraintsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}

func NewModelGetConstraintsCommandForTest() cmd.Command {
	cmd := &modelGetConstraintsCommand{}
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	return modelcmd.Wrap(cmd)
}

var GetBudgetAPIClient = &getBudgetAPIClient

// NewModelCredentialCommandForTest returns a ModelCredentialCommand with the api provided as specified.
func NewModelCredentialCommandForTest(modelClient ModelCredentialAPI, cloudClient CloudAPI, rootFunc func() (base.APICallCloser, error), store jujuclient.ClientStore) cmd.Command {
	cmd := &modelCredentialCommand{
		newModelCredentialAPIFunc: func(root base.APICallCloser) ModelCredentialAPI {
			return modelClient
		},
		newCloudAPIFunc: func(root base.APICallCloser) CloudAPI {
			return cloudClient
		},
		newAPIRootFunc: rootFunc,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
