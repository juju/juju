// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

// APIEndpoint holds information about an API endpoint.
type APIEndpoint struct {
	// APIAddress holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	Addresses []string

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

// ConfigStorage stores environment configuration data.
type ConfigStorage interface {
	// ReadInfo reads information associated with
	// the environment with the given name.
	// If there is no such information, it will
	// return an errors.NotFound error.
	ReadInfo(envName string) (EnvironInfo, error)
}

// EnvironInfo holds information associated with an environment.
type EnvironInfo interface {
	// APIEndpoint returns the current API endpoint information.
	APIEndpoint() APIEndpoint

	// APICredentials returns the current API credentials.
	APICredentials() APICredentials
}
