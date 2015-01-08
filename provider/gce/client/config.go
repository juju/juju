// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"net/mail"

	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

const (
	// These are not GCE-official environment variable names.
	OSEnvPrivateKey    = "GCE_PRIVATE_KEY"
	OSEnvClientID      = "GCE_CLIENT_ID"
	OSEnvClientEmail   = "GCE_CLIENT_EMAIL"
	OSEnvRegion        = "GCE_REGION"
	OSEnvProjectID     = "GCE_PROJECT_ID"
	OSEnvImageEndpoint = "GCE_IMAGE_URL"
)

func ValidateConnection(conn *Connection) error {
	if conn.Region == "" {
		return &config.InvalidConfigValue{Key: OSEnvRegion}
	}
	if conn.ProjectID == "" {
		return &config.InvalidConfigValue{Key: OSEnvProjectID}
	}
	return nil
}

func ValidateAuth(auth Auth) error {
	if auth.ClientID == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientID}
	}
	if auth.ClientEmail == "" {
		return &config.InvalidConfigValue{Key: OSEnvClientEmail}
	} else if _, err := mail.ParseAddress(auth.ClientEmail); err != nil {
		err = errors.Trace(err)
		return &config.InvalidConfigValue{OSEnvClientEmail, auth.ClientEmail, err}
	}
	if len(auth.PrivateKey) == 0 {
		return &config.InvalidConfigValue{Key: OSEnvPrivateKey}
	}
	return nil
}
