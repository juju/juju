// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"net/mail"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// The names of OS environment variables related to GCE.
//
// Note that these are not specified by Google. Instead they are
// defined by juju for use with the GCE provider. If Google defines
// equivalent environment variables they should be used instead.
const (
	OSEnvPrivateKey    = "GCE_PRIVATE_KEY"
	OSEnvClientID      = "GCE_CLIENT_ID"
	OSEnvClientEmail   = "GCE_CLIENT_EMAIL"
	OSEnvRegion        = "GCE_REGION"
	OSEnvProjectID     = "GCE_PROJECT_ID"
	OSEnvImageEndpoint = "GCE_IMAGE_URL"
)

// ValidateConnection checks the connection's fields for invalid values.
// If the values are not valid, it returns a config.InvalidConfigValue
// error with the key set to the corresponding OS environment variable
// name.
//
// To be considered valid, each of the connection's must be set to some
// non-empty value.
func ValidateConnection(conn *Connection) error {
	if conn.Region == "" {
		return &config.InvalidConfigValueError{Key: OSEnvRegion}
	}
	if conn.ProjectID == "" {
		return &config.InvalidConfigValueError{Key: OSEnvProjectID}
	}
	return nil
}

// ValidateAuth checks the auth's fields for invalid values.
// If the values are not valid, it returns a config.InvalidConfigValue
// error with the key set to the corresponding OS environment variable
// name.
//
// To be considered valid, each of the auth's must be set to some
// non-empty value. Furthermore, ClientEmail must be a proper email
// address.
func ValidateAuth(auth Auth) error {
	if auth.ClientID == "" {
		return &config.InvalidConfigValueError{Key: OSEnvClientID}
	}
	if auth.ClientEmail == "" {
		return &config.InvalidConfigValueError{Key: OSEnvClientEmail}
	}
	if _, err := mail.ParseAddress(auth.ClientEmail); err != nil {
		err = errors.Trace(err)
		return &config.InvalidConfigValueError{
			Key:    OSEnvClientEmail,
			Value:  auth.ClientEmail,
			Reason: err,
		}
	}
	if len(auth.PrivateKey) == 0 {
		return &config.InvalidConfigValueError{Key: OSEnvPrivateKey}
	}
	return nil
}
