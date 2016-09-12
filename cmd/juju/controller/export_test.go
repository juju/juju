// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewListControllersCommandForTest returns a listControllersCommand with the clientstore provided
// as specified.
func NewListControllersCommandForTest(testStore jujuclient.ClientStore, api func(string) ControllerAccessAPI) *listControllersCommand {
	return &listControllersCommand{
		store: testStore,
		api:   api,
	}
}

// NewShowControllerCommandForTest returns a showControllerCommand with the clientstore provided
// as specified.
func NewShowControllerCommandForTest(testStore jujuclient.ClientStore, api func(string) ControllerAccessAPI) *showControllerCommand {
	return &showControllerCommand{
		store: testStore,
		api:   api,
	}
}

type AddModelCommand struct {
	*addModelCommand
}

// NewAddModelCommandForTest returns a AddModelCommand with
// the api provided as specified.
func NewAddModelCommandForTest(
	apiRoot api.Connection,
	api AddModelAPI,
	cloudAPI CloudAPI,
	store jujuclient.ClientStore,
) (cmd.Command, *AddModelCommand) {
	c := &addModelCommand{
		apiRoot: apiRoot,
		newAddModelAPI: func(caller base.APICallCloser) AddModelAPI {
			return api
		},
		newCloudAPI: func(base.APICallCloser) CloudAPI {
			return cloudAPI
		},
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &AddModelCommand{c}
}

// NewListModelsCommandForTest returns a ListModelsCommand with the API
// and userCreds provided as specified.
func NewListModelsCommandForTest(modelAPI ModelManagerAPI, sysAPI ModelsSysAPI, store jujuclient.ClientStore) cmd.Command {
	c := &modelsCommand{
		modelAPI: modelAPI,
		sysAPI:   sysAPI,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

// NewRegisterCommandForTest returns a RegisterCommand with the function used
// to open the API connection mocked out.
func NewRegisterCommandForTest(apiOpen api.OpenFunc, listModels func(jujuclient.ClientStore, string, string) ([]base.UserModel, error), store jujuclient.ClientStore) *registerCommand {
	return &registerCommand{apiOpen: apiOpen, listModelsFunc: listModels, store: store}
}

// NewEnableDestroyControllerCommandForTest returns a enableDestroyController with the
// function used to open the API connection mocked out.
func NewEnableDestroyControllerCommandForTest(api removeBlocksAPI, store jujuclient.ClientStore) cmd.Command {
	c := &enableDestroyController{
		api: api,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

// NewDestroyCommandForTest returns a DestroyCommand with the controller and
// client endpoints mocked out.
func NewDestroyCommandForTest(
	api destroyControllerAPI,
	clientapi destroyClientAPI,
	store jujuclient.ClientStore,
	apierr error,
) cmd.Command {
	cmd := &destroyCommand{
		destroyCommandBase: destroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	}
	cmd.SetClientStore(store)
	return modelcmd.WrapController(
		cmd,
		modelcmd.WrapControllerSkipControllerFlags,
		modelcmd.WrapControllerSkipDefaultController,
	)
}

// NewKillCommandForTest returns a killCommand with the controller and client
// endpoints mocked out.
func NewKillCommandForTest(
	api destroyControllerAPI,
	clientapi destroyClientAPI,
	store jujuclient.ClientStore,
	apierr error,
	clock clock.Clock,
	apiOpen modelcmd.APIOpener,
) cmd.Command {
	kill := &killCommand{
		destroyCommandBase: destroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	}
	kill.SetClientStore(store)
	return wrapKillCommand(kill, apiOpen, clock)
}

// NewGetConfigCommandCommandForTest returns a GetConfigCommandCommand with
// the api provided as specified.
func NewGetConfigCommandForTest(api controllerAPI, store jujuclient.ClientStore) cmd.Command {
	c := &getConfigCommand{api: api}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

type CtrData ctrData
type ModelData modelData

func FmtCtrStatus(data CtrData) string {
	return fmtCtrStatus(ctrData(data))
}

func FmtModelStatus(data ModelData) string {
	return fmtModelStatus(modelData(data))
}

func NewData(api destroyControllerAPI, ctrUUID string) (ctrData, []modelData, error) {
	return newData(api, ctrUUID)
}
