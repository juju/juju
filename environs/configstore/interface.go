// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"errors"
)

// DefaultAdminUsername is used as the username to connect as in the
// absense of any explicit username being defined in the config store.
var DefaultAdminUsername = "admin"

var ErrEnvironInfoAlreadyExists = errors.New("environment info already exists")

// APIEndpoint holds information about an API endpoint.
type APIEndpoint struct {
	// APIAddress holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	Addresses []string

	// Hostnames holds a list of API addresses which may contain
	// unresolved hostnames. It's used to compare more recent API
	// addresses before resolving hostnames to determine if the cached
	// addresses have changed and therefore perform (a possibly slow)
	// local DNS resolution before comparing them against Addresses.
	Hostnames []string

	// CACert holds the CA certificate that
	// signed the API server's key.
	CACert string

	// EnvironUUID holds the UUID for the environment we are connecting to.
	// This may be empty if the environment has not been bootstrapped.
	EnvironUUID string

	// ServerUUID holds the UUID for the server environment. This may be empty
	// if the server is old and not sending the server uuid in the login
	// repsonse.
	ServerUUID string
}

// APICredentials hold credentials for connecting to an API endpoint.
type APICredentials struct {
	// User holds the name of the user to connect as.
	User     string
	Password string
}

// Storage stores environment and server configuration data.
type Storage interface {
	// ReadInfo reads information associated with
	// the environment with the given name.
	// If there is no such information, it will
	// return an errors.NotFound error.
	ReadInfo(envName string) (EnvironInfo, error)

	// CreateInfo creates some uninitialized information associated
	// with the environment with the given name.
	CreateInfo(envName string) EnvironInfo

	// List returns a slice of existing environment names that the Storage
	// knows about.
	List() ([]string, error)

	// ListSystems returns a slice of existing server names that the Storage
	// knows about.
	ListSystems() ([]string, error)
}

// EnvironInfo holds information associated with an environment.
type EnvironInfo interface {
	// Initialized returns whether the environment information has
	// been initialized. It will return true for EnvironInfo instances
	// that have been created but not written.
	Initialized() bool

	// BootstrapConfig returns the configuration attributes
	// that an environment will be bootstrapped with.
	BootstrapConfig() map[string]interface{}

	// APIEndpoint returns the current API endpoint information.
	APIEndpoint() APIEndpoint

	// APICredentials returns the current API credentials.
	APICredentials() APICredentials

	// SetBootstrapConfig sets the configuration attributes
	// to be used for bootstrapping.
	// This method may only be called on an EnvironInfo
	// obtained using ConfigStorage.CreateInfo.
	SetBootstrapConfig(map[string]interface{})

	// SetAPIEndpoint sets the API endpoint information
	// currently associated with the environment.
	SetAPIEndpoint(APIEndpoint)

	// SetAPICreds sets the API credentials currently
	// associated with the environment.
	SetAPICredentials(APICredentials)

	// Location returns the location of the source of the environment
	// information in a human readable format.
	Location() string

	// Write writes the current information to persistent storage. A
	// subsequent call to ConfigStorage.ReadInfo can retrieve it. After this
	// call succeeds, Initialized will return true.
	// It return ErrAlreadyExists if the EnvironInfo is not yet Initialized
	// and the EnvironInfo has been written before.
	Write() error

	// Destroy destroys the information associated with
	// the environment.
	Destroy() error
}
