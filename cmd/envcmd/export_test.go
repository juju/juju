// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

var (
	GetCurrentEnvironmentFilePath = getCurrentEnvironmentFilePath
	GetCurrentSystemFilePath      = getCurrentSystemFilePath
	GetConfigStore                = &getConfigStore
	EndpointRefresher             = &endpointRefresher
)

// NewEnvCommandBase returns a new EnvCommandBase with the environment name, client,
// and error as specified for testing purposes.
// If getterErr != nil then the NewEnvironmentGetter returns the specified error.
func NewEnvCommandBase(name string, client EnvironmentGetter, getterErr error) *EnvCommandBase {
	return &EnvCommandBase{
		envName:         name,
		envGetterClient: client,
		envGetterErr:    getterErr,
	}
}
