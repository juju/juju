// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
)

// NewListCommandForTest returns a ListCommand with the configstore provided
// as specified.
func NewListCommandForTest(cfgStore configstore.Storage) *listCommand {
	return &listCommand{
		cfgStore: cfgStore,
	}
}

type CreateModelCommand struct {
	*createModelCommand
}

// NewCreateModelCommandForTest returns a CreateModelCommand with
// the api provided as specified.
func NewCreateModelCommandForTest(api CreateEnvironmentAPI, parser func(interface{}) (interface{}, error)) (cmd.Command, *CreateModelCommand) {
	c := &createModelCommand{
		api:          api,
		configParser: parser,
	}
	return modelcmd.WrapController(c), &CreateModelCommand{c}
}

// NewModelsCommandForTest returns a EnvironmentsCommand with the API
// and userCreds provided as specified.
func NewModelsCommandForTest(modelAPI ModelManagerAPI, sysAPI ModelsSysAPI, userCreds *configstore.APICredentials) cmd.Command {
	return modelcmd.WrapController(&environmentsCommand{
		modelAPI:  modelAPI,
		sysAPI:    sysAPI,
		userCreds: userCreds,
	})
}

// NewLoginCommandForTest returns a LoginCommand with the function used to open
// the API connection mocked out.
func NewLoginCommandForTest(apiOpen api.OpenFunc, getUserManager GetUserManagerFunc) *loginCommand {
	return &loginCommand{
		loginAPIOpen:   apiOpen,
		GetUserManager: getUserManager,
	}
}

// NewRegisterCommandForTest returns a RegisterCommand with the function used
// to open the API connection mocked out.
func NewRegisterCommandForTest(apiOpen api.OpenFunc, newAPIRoot modelcmd.OpenFunc) *registerCommand {
	return &registerCommand{apiOpen: apiOpen, newAPIRoot: newAPIRoot}
}

type UseModelCommand struct {
	*useModelCommand
}

// NewUseModelCommandForTest returns a UseModelCommand with the
// API and userCreds provided as specified.
func NewUseModelCommandForTest(api UseModelAPI, userCreds *configstore.APICredentials, endpoint *configstore.APIEndpoint) (cmd.Command, *UseModelCommand) {
	c := &useModelCommand{
		api:       api,
		userCreds: userCreds,
		endpoint:  endpoint,
	}
	return modelcmd.WrapController(c), &UseModelCommand{c}
}

// NewRemoveBlocksCommandForTest returns a RemoveBlocksCommand with the
// function used to open the API connection mocked out.
func NewRemoveBlocksCommandForTest(api removeBlocksAPI) cmd.Command {
	return modelcmd.WrapController(&removeBlocksCommand{
		api: api,
	})
}

// NewDestroyCommandForTest returns a DestroyCommand with the controller and
// client endpoints mocked out.
func NewDestroyCommandForTest(api destroyControllerAPI, clientapi destroyClientAPI, apierr error) cmd.Command {
	return modelcmd.WrapController(
		&destroyCommand{
			destroyCommandBase: destroyCommandBase{
				api:       api,
				clientapi: clientapi,
				apierr:    apierr,
			},
		},
		modelcmd.ControllerSkipFlags,
		modelcmd.ControllerSkipDefault,
	)
}

// NewKillCommandForTest returns a killCommand with the controller and client
// endpoints mocked out.
func NewKillCommandForTest(
	api destroyControllerAPI,
	clientapi destroyClientAPI,
	apierr error,
	clock clock.Clock,
	apiOpenFunc func(string) (api.Connection, error),
) cmd.Command {
	kill := &killCommand{
		destroyCommandBase: destroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	}
	return wrapKillCommand(kill, apiOpenFunc, clock)
}

// NewListBlocksCommandForTest returns a ListBlocksCommand with the controller
// endpoint mocked out.
func NewListBlocksCommandForTest(api listBlocksAPI, apierr error) cmd.Command {
	return modelcmd.WrapController(&listBlocksCommand{
		api:    api,
		apierr: apierr,
	})
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
