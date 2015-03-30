// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"encoding/json"
	"io"
	"net/mail"

	"code.google.com/p/google-api-go-client/compute/v1"
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

// ParseAuthFile extracts the auth information from the JSON file
// downloaded from the GCE console (under /apiui/credential).
func ParseAuthFile(authFile io.Reader) (map[string]string, error) {
	data := make(map[string]string)
	if err := json.NewDecoder(authFile).Decode(&data); err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range data {
		switch k {
		case "private_key":
			data[OSEnvPrivateKey] = v
			delete(data, k)
		case "client_email":
			data[OSEnvClientEmail] = v
			delete(data, k)
		case "client_id":
			data[OSEnvClientID] = v
			delete(data, k)
		}
	}
	return data, nil
}

// Credentials holds the OAuth2 credentials needed to authenticate on GCE.
type Credentials struct {
	// ClientID is the GCE account's OAuth ID. It is part of the OAuth
	// config used in the OAuth-wrapping network transport.
	ClientID string

	// ClientEmail is the email address associatd with the GCE account.
	// It is used to generate a new OAuth token to use in the
	// OAuth-wrapping network transport.
	ClientEmail string

	// PrivateKey is the private key that matches the public key
	// associatd with the GCE account. It is used to generate a new
	// OAuth token to use in the OAuth-wrapping network transport.
	PrivateKey []byte
}

// Validate checks the credentialss for invalid values. If the values
// are not valid, it returns errors.NotValid with the message set to
// the corresponding OS environment variable name.
//
// To be considered valid, each of the credentials must be set to some
// non-empty value. Furthermore, ClientEmail must be a proper email
// address.
func (gc Credentials) Validate() error {
	if gc.ClientID == "" {
		return NewInvalidCredential(OSEnvClientID, "", "missing ClientID")
	}
	if gc.ClientEmail == "" {
		return NewInvalidCredential(OSEnvClientEmail, "", "missing ClientEmail")
	}
	if _, err := mail.ParseAddress(gc.ClientEmail); err != nil {
		return NewInvalidCredential(OSEnvClientEmail, gc.ClientEmail, err)
	}
	if len(gc.PrivateKey) == 0 {
		return NewInvalidCredential(OSEnvPrivateKey, "", "missing PrivateKey")
	}
	return nil
}

// ConnectionConfig contains the config values used for a connection
// to the GCE API.
type ConnectionConfig struct {
	// Region is the GCE region in which to operate for the connection.
	Region string

	// ProjectID is the project ID to use in all GCE API requests for
	// the connection.
	ProjectID string
}

// Validate checks the connection's fields for invalid values.
// If the values are not valid, it returns a config.InvalidConfigValue
// error with the key set to the corresponding OS environment variable
// name.
//
// To be considered valid, each of the connection's must be set to some
// non-empty value.
func (gc ConnectionConfig) Validate() error {
	if gc.Region == "" {
		return &config.InvalidConfigValueError{Key: OSEnvRegion}
	}
	if gc.ProjectID == "" {
		return &config.InvalidConfigValueError{Key: OSEnvProjectID}
	}
	return nil
}

// Connect authenticates using the provided credentials and opens a
// low-level connection to the GCE API for the Connection. Calling
// Connect after a successful connection has already been made will
// result in an error. All errors that happen while authenticating and
// connecting are returned by Connect.
func (gc ConnectionConfig) Connect(creds Credentials) (*Connection, error) {
	raw, err := newRawConnection(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	conn := &Connection{
		raw:       &rawConn{raw},
		Region:    gc.Region,
		ProjectID: gc.ProjectID,
	}
	return conn, nil
}

var newRawConnection = func(creds Credentials) (*compute.Service, error) {
	return newConnection(creds)
}
