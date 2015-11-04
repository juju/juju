// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

var (
	SetConfigSpecialCaseDefaults = setConfigSpecialCaseDefaults
	UserCurrent                  = &userCurrent
)

// NewListCommand returns a ListCommand with the configstore provided as specified.
func NewListCommand(cfgStore configstore.Storage) *listCommand {
	return &listCommand{
		cfgStore: cfgStore,
	}
}

type CreateEnvironmentCommand struct {
	*createEnvironmentCommand
}

// NewCreateEnvironmentCommand returns a CreateEnvironmentCommand with the api provided as specified.
func NewCreateEnvironmentCommand(api CreateEnvironmentAPI, parser func(interface{}) (interface{}, error)) (cmd.Command, *CreateEnvironmentCommand) {
	c := &createEnvironmentCommand{
		api:          api,
		configParser: parser,
	}
	return envcmd.WrapController(c), &CreateEnvironmentCommand{c}
}

// NewEnvironmentsCommand returns a EnvironmentsCommand with the API and userCreds
// provided as specified.
func NewEnvironmentsCommand(envAPI EnvironmentsEnvAPI, sysAPI EnvironmentsSysAPI, userCreds *configstore.APICredentials) cmd.Command {
	return envcmd.WrapController(&environmentsCommand{
		envAPI:    envAPI,
		sysAPI:    sysAPI,
		userCreds: userCreds,
	})
}

// NewLoginCommand returns a LoginCommand with the function used to open
// the API connection mocked out.
func NewLoginCommand(apiOpen api.OpenFunc, getUserManager GetUserManagerFunc) *loginCommand {
	return &loginCommand{
		loginAPIOpen:   apiOpen,
		GetUserManager: getUserManager,
	}
}

type UseEnvironmentCommand struct {
	*useEnvironmentCommand
}

// NewUseEnvironmentCommand returns a UseEnvironmentCommand with the API and
// userCreds provided as specified.
func NewUseEnvironmentCommand(api UseEnvironmentAPI, userCreds *configstore.APICredentials, endpoint *configstore.APIEndpoint) (cmd.Command, *UseEnvironmentCommand) {
	c := &useEnvironmentCommand{
		api:       api,
		userCreds: userCreds,
		endpoint:  endpoint,
	}
	return envcmd.WrapController(c), &UseEnvironmentCommand{c}
}

// NewRemoveBlocksCommand returns a RemoveBlocksCommand with the function used
// to open the API connection mocked out.
func NewRemoveBlocksCommand(api removeBlocksAPI) cmd.Command {
	return envcmd.WrapController(&removeBlocksCommand{
		api: api,
	})
}

// NewDestroyCommand returns a DestroyCommand with the controller and client
// endpoints mocked out.
func NewDestroyCommand(api destroyControllerAPI, clientapi destroyClientAPI, apierr error) cmd.Command {
	return envcmd.WrapBase(&destroyCommand{
		destroyCommandBase: destroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	})
}

// NewKillCommand returns a killCommand with the controller and client
// endpoints mocked out.
func NewKillCommand(api destroyControllerAPI,
	clientapi destroyClientAPI,
	apierr error,
	dialFunc func(string) (api.Connection, error)) cmd.Command {
	return envcmd.WrapBase(&killCommand{
		destroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
		dialFunc,
	})
}

// NewListBlocksCommand returns a ListBlocksCommand with the controller
// endpoint mocked out.
func NewListBlocksCommand(api listBlocksAPI, apierr error) cmd.Command {
	return envcmd.WrapController(&listBlocksCommand{
		api:    api,
		apierr: apierr,
	})
}
