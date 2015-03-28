// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/mail"

	"github.com/juju/errors"
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

const (
	jsonKeyTypeServiceAccount = "service_account"
)

// Credentials holds the OAuth2 credentials needed to authenticate on GCE.
type Credentials struct {
	// JSONKey is the content of the JSON key file for these credentials.
	JSONKey []byte

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

// NewCredentials returns a new Credentials based on the provided
// values. The keys must be recognized OS env var names for the
// different credential fields.
func NewCredentials(values map[string]string) (*Credentials, error) {
	var creds Credentials
	for k, v := range values {
		switch k {
		case OSEnvClientID:
			creds.ClientID = v
		case OSEnvClientEmail:
			creds.ClientEmail = v
		case OSEnvPrivateKey:
			creds.PrivateKey = []byte(v)
		default:
			return nil, errors.NotSupportedf("key %q", k)
		}
	}

	if err := creds.Validate(); err == nil {
		jk, err := creds.buildJSONKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		creds.JSONKey = jk
	}

	return &creds, nil
}

// ParseJSONKey returns a new Credentials with values based on the
// provided JSON key file contents.
func ParseJSONKey(jsonKeyFile io.Reader) (*Credentials, error) {
	jsonKey, err := ioutil.ReadAll(jsonKeyFile)
	if err != nil {
		return nil, errors.Trace(err)
	}
	values, err := parseJSONKey(jsonKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	delete(values, "type")
	delete(values, "private_key_id")
	creds, err := NewCredentials(values)
	if err != nil {
		return nil, errors.Trace(err)
	}
	creds.JSONKey = jsonKey
	return creds, nil
}

// parseJSONKey extracts the auth information from the JSON file
// downloaded from the GCE console (under /apiui/credential).
func parseJSONKey(jsonKey []byte) (map[string]string, error) {
	data := make(map[string]string)
	if err := json.Unmarshal(jsonKey, &data); err != nil {
		return nil, errors.Trace(err)
	}

	keyType, ok := data["type"]
	if !ok {
		return nil, errors.New(`missing "type"`)
	}
	switch keyType {
	case jsonKeyTypeServiceAccount:
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
	default:
		return nil, errors.NotSupportedf("JSON key type %q", data["type"])
	}
	return data, nil
}

// buildJSONKey returns the content of the JSON key file for the
// credential values.
func (gc Credentials) buildJSONKey() ([]byte, error) {
	return json.Marshal(&map[string]string{
		"type":         jsonKeyTypeServiceAccount,
		"client_id":    gc.ClientID,
		"client_email": gc.ClientEmail,
		"private_key":  string(gc.PrivateKey),
	})
}

// Values returns the credentials as a simple mapping with the
// corresponding OS env variable names as the keys.
func (gc Credentials) Values() map[string]string {
	return map[string]string{
		OSEnvClientID:    gc.ClientID,
		OSEnvClientEmail: gc.ClientEmail,
		OSEnvPrivateKey:  string(gc.PrivateKey),
	}
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
		return NewMissingConfigValue(OSEnvClientID, "ClientID")
	}
	if gc.ClientEmail == "" {
		return NewMissingConfigValue(OSEnvClientEmail, "ClientEmail")
	}
	if _, err := mail.ParseAddress(gc.ClientEmail); err != nil {
		return NewInvalidConfigValue(OSEnvClientEmail, gc.ClientEmail, err)
	}
	if len(gc.PrivateKey) == 0 {
		return NewMissingConfigValue(OSEnvPrivateKey, "PrivateKey")
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
		return NewMissingConfigValue(OSEnvRegion, "Region")
	}
	if gc.ProjectID == "" {
		return NewMissingConfigValue(OSEnvProjectID, "ProjectID")
	}
	return nil
}
