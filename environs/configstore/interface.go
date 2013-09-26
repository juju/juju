// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package configstore

import (
	"errors"
)

var ErrEnvironInfoAlreadyExists = errors.New("environment info already exists")

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

// Storage stores environment configuration data.
type Storage interface {
	// ReadInfo reads information associated with
	// the environment with the given name.
	// If there is no such information, it will
	// return an errors.NotFound error.
	ReadInfo(envName string) (EnvironInfo, error)

	// CreateInfo creates some uninitialized information associated
	// with the environment with the given name.
	// It return ErrAlreadyExists if the
	// information has already been created.
	CreateInfo(envName string) (EnvironInfo, error)
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

	// Write writes the current information to persistent storage.
	// A subsequent call to ConfigStorage.ReadInfo
	// can retrieve it. After this call succeeds, Initialized will return true.
	Write() error

	// Destroy destroys the information associated with
	// the environment.
	Destroy() error
}
