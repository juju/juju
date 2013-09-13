// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs
import (
	"launchpad.net/juju-core/environs/config"
)

// APIEndpoint holds information about an API endpoint.
type APIEndpoint struct {
	// APIAddress holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	APIAddresses []string

	// CACert holds the CA certificate that
	// signed the API server's key.
	CACert []byte
}

// APICredentials hold credentials for connecting to an API endpoint.
type APICredentials struct {
	// User holds the name of the user to connect as.
	User string
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

	// ExtraConfig holds any environ.Config attributes that have
	// been added by Prepare. When an environment is fully
	// bootstrapped, and its secrets pushed, this field is optional,
	// although it is still potentially useful if all the API
	// endpoint addresses have changed - it can be used to create an
	// Environ to retrieve a current StateInfo.
	ExtraConfig map[string]interface{}

	// NeedSecrets stores whether the environment's
	// secrets have yet been pushed up with the first
	// state connection.
	NeedSecrets bool
}

// ConfigStorage stores environment configuration data.
type ConfigStorage interface {
	// DefaultName returns the name of the default environment to use.
	DefaultName() (string, error)

	// EnvironConfig returns base environment configuration for the
	// environment with the given name. The configuration does not
	// include attributes added by the environment when it is
	// prepared. Conventionally EnvironConfig will return data read
	// from $HOME/.juju/environments.yaml.
	EnvironConfig(envName string) (*config.Config, error)

	// EnvironInfo returns information on the environment with the
	// given name, previously stored with WriteEnvironInfo.
	// Conventionally EnvironInfo will return data read from
	// $HOME/.juju/.environments/$envName.yaml.
	EnvironInfo(envName string) (*EnvironInfo, error)

	// WriteEnvironInfo writes information on the environment with
	// the given name. Conventionally EnvironInfo will write to
	// $HOME/.juju/.environments/$envName.yaml.
	WriteEnvironInfo(envName string, info *EnvironInfo) error
}
