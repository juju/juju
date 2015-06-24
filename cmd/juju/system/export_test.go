// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/environs/configstore"
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
func NewEnvironmentsCommand(api EnvironmentManagerAPI, userCreds *configstore.APICredentials) *EnvironmentsCommand {
	return &EnvironmentsCommand{
		api:       api,
		userCreds: userCreds,
	}
}

// NewLoginCommand returns a LoginCommand with the function used to open
// the API connection mocked out.
func NewLoginCommand(apiOpen APIOpenFunc, getUserManager GetUserManagerFunc) *LoginCommand {
	return &LoginCommand{
		apiOpen:        apiOpen,
		getUserManager: getUserManager,
	}
}

// NewUseEnvironmentCommand returns a UseEnvironmentCommand with the API and
// userCreds provided as specified.
func NewUseEnvironmentCommand(api EnvironmentManagerAPI, userCreds *configstore.APICredentials, endpoint *configstore.APIEndpoint) *UseEnvironmentCommand {
	return &UseEnvironmentCommand{
		api:       api,
		userCreds: userCreds,
		endpoint:  endpoint,
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

// NewDestroyCommand returns a DestroyCommand with the the environmentmanager and client
// endpoints mocked out.
func NewDestroyCommand(api destroyEnvironmentAPI, clientapi destroyEnvironmentClientAPI, apierr error) *DestroyCommand {
	return &DestroyCommand{
		api:       api,
		clientapi: clientapi,
		apierr:    apierr,
	}
}

// NewForceDestroyCommand returns a ForceDestroyCommand with the the environmentmanager and client
// endpoints mocked out.
func NewForceDestroyCommand(api destroyEnvironmentAPI, clientapi destroyEnvironmentClientAPI, apierr error) *ForceDestroyCommand {
	return &ForceDestroyCommand{
		DestroyCommand: DestroyCommand{
			api:       api,
			clientapi: clientapi,
			apierr:    apierr,
		},
	}
}
