// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/configstore"
)

var (
	SetConfigSpecialCaseDefaults = setConfigSpecialCaseDefaults
	UserCurrent                  = &userCurrent
)

// NewListCommand returns a ListCommand with the configstore provided as specified.
func NewListCommand(cfgStore configstore.Storage) *ListCommand {
	return &ListCommand{
		cfgStore: cfgStore,
	}
}

// NewCreateEnvironmentCommand returns a CreateEnvironmentCommand with the api provided as specified.
func NewCreateEnvironmentCommand(api CreateEnvironmentAPI) *CreateEnvironmentCommand {
	return &CreateEnvironmentCommand{
		api: api,
	}
}

// NewEnvironmentsCommand returns a EnvironmentsCommand with the API and userCreds
// provided as specified.
func NewEnvironmentsCommand(envAPI EnvironmentsEnvAPI, sysAPI EnvironmentsSysAPI, userCreds *configstore.APICredentials) *EnvironmentsCommand {
	return &EnvironmentsCommand{
		envAPI:    envAPI,
		sysAPI:    sysAPI,
		userCreds: userCreds,
	}
}

// NewLoginCommand returns a LoginCommand with the function used to open
// the API connection mocked out.
func NewLoginCommand(apiOpen api.OpenFunc, getUserManager GetUserManagerFunc) *LoginCommand {
	return &LoginCommand{
		apiOpen:        apiOpen,
		getUserManager: getUserManager,
	}
}

// NewUseEnvironmentCommand returns a UseEnvironmentCommand with the API and
// userCreds provided as specified.
func NewUseEnvironmentCommand(api UseEnvironmentAPI, userCreds *configstore.APICredentials, endpoint *configstore.APIEndpoint) *UseEnvironmentCommand {
	return &UseEnvironmentCommand{
		api:       api,
		userCreds: userCreds,
		endpoint:  endpoint,
	}
}

// NewRemoveBlocksCommand returns a RemoveBlocksCommand with the function used
// to open the API connection mocked out.
func NewRemoveBlocksCommand(api removeBlocksAPI) *RemoveBlocksCommand {
	return &RemoveBlocksCommand{
		api: api,
	}
}

// Name makes the private name attribute accessible for tests.
func (c *CreateEnvironmentCommand) Name() string {
	return c.name
}

// Owner makes the private name attribute accessible for tests.
func (c *CreateEnvironmentCommand) Owner() string {
	return c.owner
}

// ConfigFile makes the private configFile attribute accessible for tests.
func (c *CreateEnvironmentCommand) ConfigFile() cmd.FileVar {
	return c.configFile
}

// ConfValues makes the private confValues attribute accessible for tests.
func (c *CreateEnvironmentCommand) ConfValues() map[string]string {
	return c.confValues
}

// NewDestroyCommand returns a DestroyCommand with the systemmanager and client
// endpoints mocked out.
func NewDestroyCommand(api destroySystemAPI, clientapi destroyClientAPI, apierr error) *DestroyCommand {
	return &DestroyCommand{
		DestroyCommandBase: DestroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	}
}

// NewKillCommand returns a KillCommand with the systemmanager and client
// endpoints mocked out.
func NewKillCommand(api destroySystemAPI, clientapi destroyClientAPI, apierr error, dialFunc func(string) (api.Connection, error)) *KillCommand {
	return &KillCommand{
		DestroyCommandBase{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
		dialFunc,
	}
}

// NewListBlocksCommand returns a ListBlocksCommand with the systemmanager
// endpoint mocked out.
func NewListBlocksCommand(api listBlocksAPI, apierr error) *ListBlocksCommand {
	return &ListBlocksCommand{
		api:    api,
		apierr: apierr,
	}
}
