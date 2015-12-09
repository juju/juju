// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

var (
	SetConfigSpecialCaseDefaults = setConfigSpecialCaseDefaults
	UserCurrent                  = &userCurrent
)

// NewListCommandForTest returns a ListCommand with the configstore provided
// as specified.
func NewListCommandForTest(cfgStore configstore.Storage) *listCommand {
	return &listCommand{
		cfgStore: cfgStore,
	}
}

type CreateEnvironmentCommand struct {
	*createEnvironmentCommand
}

// NewCreateEnvironmentCommandForTest returns a CreateEnvironmentCommand with
// the api provided as specified.
func NewCreateEnvironmentCommandForTest(api CreateEnvironmentAPI, parser func(interface{}) (interface{}, error)) (cmd.Command, *CreateEnvironmentCommand) {
	c := &createEnvironmentCommand{
		api:          api,
		configParser: parser,
	}
	return envcmd.WrapController(c), &CreateEnvironmentCommand{c}
}

// NewEnvironmentsCommandForTest returns a EnvironmentsCommand with the API
// and userCreds provided as specified.
func NewEnvironmentsCommandForTest(envAPI EnvironmentsEnvAPI, sysAPI EnvironmentsSysAPI, userCreds *configstore.APICredentials) cmd.Command {
	return envcmd.WrapController(&environmentsCommand{
		envAPI:    envAPI,
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

type UseEnvironmentCommand struct {
	*useEnvironmentCommand
}

// NewUseEnvironmentCommandForTest returns a UseEnvironmentCommand with the
// API and userCreds provided as specified.
func NewUseEnvironmentCommandForTest(api UseEnvironmentAPI, userCreds *configstore.APICredentials, endpoint *configstore.APIEndpoint) (cmd.Command, *UseEnvironmentCommand) {
	c := &useEnvironmentCommand{
		api:       api,
		userCreds: userCreds,
		endpoint:  endpoint,
	}
	return envcmd.WrapController(c), &UseEnvironmentCommand{c}
}

// NewRemoveBlocksCommandForTest returns a RemoveBlocksCommand with the
// function used to open the API connection mocked out.
func NewRemoveBlocksCommandForTest(api removeBlocksAPI) cmd.Command {
	return envcmd.WrapController(&removeBlocksCommand{
		api: api,
	})
}

// NewDestroyCommandForTest returns a DestroyCommand with the controller and
// client endpoints mocked out.
func NewDestroyCommandForTest(api destroyControllerAPI, clientapi destroyClientAPI, apierr error) cmd.Command {
	return envcmd.Wrap(
		&destroyCommand{
			destroyCommandBase: destroyCommandBase{
				api:       api,
				clientapi: clientapi,
				apierr:    apierr,
			},
		},
		envcmd.EnvSkipFlags,
		envcmd.EnvSkipDefault,
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
	return envcmd.WrapController(&listBlocksCommand{
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
