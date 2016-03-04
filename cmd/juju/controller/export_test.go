// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
)

// NewListControllersCommandForTest returns a listControllersCommand with the clientstore provided
// as specified.
func NewListControllersCommandForTest(testStore jujuclient.ClientStore) *listControllersCommand {
	return &listControllersCommand{
		store: testStore,
	}
}

// NewShowControllerCommandForTest returns a showControllerCommand with the clientstore provided
// as specified.
func NewShowControllerCommandForTest(testStore jujuclient.ClientStore) *showControllerCommand {
	return &showControllerCommand{
		store: testStore,
	}
}

type CreateModelCommand struct {
	*createModelCommand
}

// NewCreateModelCommandForTest returns a CreateModelCommand with
// the api provided as specified.
func NewCreateModelCommandForTest(
	api CreateEnvironmentAPI,
	store jujuclient.ClientStore,
	parser func(interface{}) (interface{}, error),
) (cmd.Command, *CreateModelCommand) {
	c := &createModelCommand{
		api:          api,
		configParser: parser,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c), &CreateModelCommand{c}
}

// NewListModelsCommandForTest returns a EnvironmentsCommand with the API
// and userCreds provided as specified.
func NewListModelsCommandForTest(modelAPI ModelManagerAPI, sysAPI ModelsSysAPI, store jujuclient.ClientStore, userCreds *configstore.APICredentials) cmd.Command {
	c := &modelsCommand{
		modelAPI:  modelAPI,
		sysAPI:    sysAPI,
		userCreds: userCreds,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

// NewRegisterCommandForTest returns a RegisterCommand with the function used
// to open the API connection mocked out.
func NewRegisterCommandForTest(apiOpen api.OpenFunc, newAPIRoot modelcmd.OpenFunc, store jujuclient.ClientStore) *registerCommand {
	return &registerCommand{apiOpen: apiOpen, newAPIRoot: newAPIRoot, store: store}
}

// NewRemoveBlocksCommandForTest returns a RemoveBlocksCommand with the
// function used to open the API connection mocked out.
func NewRemoveBlocksCommandForTest(api removeBlocksAPI, store jujuclient.ClientStore) cmd.Command {
	c := &removeBlocksCommand{
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
		modelcmd.ControllerSkipFlags,
		modelcmd.ControllerSkipDefault,
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

// NewListBlocksCommandForTest returns a ListBlocksCommand with the controller
// endpoint mocked out.
func NewListBlocksCommandForTest(api listBlocksAPI, apierr error, store jujuclient.ClientStore) cmd.Command {
	c := &listBlocksCommand{
		api:    api,
		apierr: apierr,
	}
	c.SetClientStore(store)
	return modelcmd.WrapController(c)
}

type CtrData ctrData
type EnvData envData

func FmtCtrStatus(data CtrData) string {
	return fmtCtrStatus(ctrData(data))
}

func FmtEnvStatus(data EnvData) string {
	return fmtEnvStatus(envData(data))
}

func NewData(api destroyControllerAPI, ctrUUID string) (ctrData, []envData, error) {
	return newData(api, ctrUUID)
}
