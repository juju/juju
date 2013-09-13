// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

// APIEndpoint holds information about an API endpoint.
type APIEndpoint struct {
	// APIAddress holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	APIAddresses []string

	// CACert holds the CA certificate that
	// signed the API server's key.
	CACert string
}

// APICredentials hold credentials for connecting to an API endpoint.
type APICredentials struct {
	// User holds the name of the user to connect as.
	User     string
	Password string
}

// EnvironInfo holds information on a given environment, to be stored
// outside that environment.
type EnvironInfo struct {
	// Creds holds information on the user to connect as.
	Creds APICredentials

	// Endpoint holds the latest information on the API endpoint. It
	// will be updated when new information becomes available.
	Endpoint APIEndpoint
}

// ConfigStorage stores environment configuration data.
type ConfigStorage interface {
	// EnvironInfo returns information on the environment with the
	// given name, previously stored with WriteEnvironInfo.
	// If there is no environment info available, an errors.NotFoundError
	// is returned.
	// Conventionally EnvironInfo will return data read from
	// $HOME/.juju/.environments/$envName.yaml.
	EnvironInfo(envName string) (*EnvironInfo, error)

	// WriteEnvironInfo writes information on the environment with
	// the given name. Conventionally EnvironInfo will write to
	// $HOME/.juju/.environments/$envName.yaml.
	WriteEnvironInfo(envName string, info *EnvironInfo) error
}
