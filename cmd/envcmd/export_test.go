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
func NewEnvCommandBase(name string, client EnvironmentGetter, err error) *EnvCommandBase {
	return &EnvCommandBase{
		envName:         name,
		envGetterClient: client,
		envGetterErr:    err,
	}
}
