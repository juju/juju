// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/juju/environs/configstore"
)

// NewListCommand returns a ListCommand with the configstore provided as specified.
func NewListCommand(cfgStore configstore.Storage) *ListCommand {
	return &ListCommand{
		cfgStore: cfgStore,
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
