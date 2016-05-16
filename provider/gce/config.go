// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce/google"
)

// TODO(ericsnow) While not strictly config-related, we could use some
// mechanism by which we can validate the values we've hard-coded in
// this provider match up with the external authoritative sources. One
// example of this is the data stored in instancetypes.go. Similarly
// we should also ensure the cloud-images metadata is correct and
// up-to-date, though that is more the responsibility of that team.
// Regardless, it may be useful to include a tool somewhere in juju
// that we can use to validate this provider's potentially out-of-date
// data.

// The GCE-specific config keys.
const (
	cfgPrivateKey    = "private-key"
	cfgClientID      = "client-id"
	cfgClientEmail   = "client-email"
	cfgRegion        = "region"
	cfgProjectID     = "project-id"
	cfgImageEndpoint = "image-endpoint"
)

var configSchema = environschema.Fields{
	cfgPrivateKey: {
		Type: environschema.Tstring,
		Description: cfgPrivateKey + ` is the private key that matches the public key
associated with the GCE account.`,
		EnvVar:    google.OSEnvPrivateKey,
		Group:     environschema.AccountGroup,
		Secret:    true,
		Mandatory: true,
	},
	cfgClientID: {
		Type:        environschema.Tstring,
		Description: cfgClientID + ` is the GCE account's OAuth ID.`,
		EnvVar:      google.OSEnvClientID,
		Group:       environschema.AccountGroup,
		Mandatory:   true,
	},
	cfgClientEmail: {
		Type:        environschema.Tstring,
		Description: cfgClientEmail + ` is the email address associated with the GCE account.`,
		EnvVar:      google.OSEnvClientEmail,
		Group:       environschema.AccountGroup,
		Mandatory:   true,
	},
	cfgProjectID: {
		Type:        environschema.Tstring,
		Description: cfgProjectID + ` is the project ID to use in all GCE API requests`,
		EnvVar:      google.OSEnvProjectID,
		Group:       environschema.AccountGroup,
		Mandatory:   true,
	},
	cfgRegion: {
		Type:        environschema.Tstring,
		Description: cfgRegion + ` is the GCE region in which to operate`,
		EnvVar:      google.OSEnvRegion,
	},
	cfgImageEndpoint: {
		Type:        environschema.Tstring,
		Description: cfgImageEndpoint + ` identifies where the provider should look for cloud images (i.e. for simplestreams)`,
		EnvVar:      google.OSEnvImageEndpoint,
	},
}

var osEnvFields = func() map[string]string {
	m := make(map[string]string)
	for name, f := range configSchema {
		if f.EnvVar != "" {
			m[f.EnvVar] = name
		}
	}
	return m
}()

// configFields is the spec for each GCE config value's type.
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

// TODO(ericsnow) Do we need custom defaults for "image-metadata-url" or
// "agent-metadata-url"? The defaults are the official ones (e.g.
// cloud-images).

var configDefaults = schema.Defaults{
	// See http://cloud-images.ubuntu.com/releases/streams/v1/com.ubuntu.cloud:released:gce.json
	cfgImageEndpoint: "https://www.googleapis.com",
	cfgRegion:        "us-central1",
}

var configImmutableFields = []string{
	cfgPrivateKey,
	cfgClientID,
	cfgClientEmail,
	cfgRegion,
	cfgProjectID,
	cfgImageEndpoint,
}

type environConfig struct {
	config      *config.Config
	attrs       map[string]interface{}
	credentials google.Credentials
	conn        google.ConnectionConfig
}

// newConfig builds a new environConfig from the provided Config
// filling in default values, if any. It returns an error if the
// resulting configuration is not valid.
func newConfig(cfg, old *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We don't allow an empty string for any attribute.
	for attr, value := range attrs {
		if value == "" {
			return nil, errors.Errorf("%s: must not be empty", attr)
		}
	}
	newCfg, err := cfg.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	credentials, err := parseCredentials(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ecfg := &environConfig{
		config:      newCfg,
		attrs:       attrs,
		credentials: *credentials,
		conn: google.ConnectionConfig{
			Region:    attrs[cfgRegion].(string),
			ProjectID: attrs[cfgProjectID].(string),
		},
	}
	// Verify that the connection object is valid.
	if err := ecfg.conn.Validate(); err != nil {
		return nil, errors.Trace(handleInvalidFieldError(err))
	}
	if old == nil {
		return ecfg, nil
	}
	// There's an old configuration. Validate it so that any
	// default values are correctly coerced for when we
	// check the old values later.
	oldEcfg, err := newConfig(old, nil)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid base config")
	}
	for _, attr := range configImmutableFields {
		oldv, newv := oldEcfg.attrs[attr], ecfg.attrs[attr]
		if oldv != newv {
			return nil, errors.Errorf("%s: cannot change from %v to %v", attr, oldv, newv)
		}
	}
	return ecfg, nil
}

func (c *environConfig) region() string {
	return c.conn.Region
}

// imageEndpoint identifies where the provider should look for
// cloud images (i.e. for simplestreams).
func (c *environConfig) imageEndpoint() string {
	return c.attrs[cfgImageEndpoint].(string)
}

// secret returns the secret configuration values.
func (c *environConfig) secret() map[string]string {
	secretAttrs := make(map[string]string)
	for attr, val := range c.attrs {
		if configSchema[attr].Secret {
			secretAttrs[attr] = val.(string)
		}
	}
	return secretAttrs
}

// parseCredentials extracts the OAuth2 info from the config from the
// individual fields (falling back on the JSON file).
func parseCredentials(attrs map[string]interface{}) (*google.Credentials, error) {
	values := make(map[string]string)
	for attr, val := range attrs {
		f := configSchema[attr]
		if f.Group == environschema.AccountGroup && f.EnvVar != "" {
			values[f.EnvVar] = val.(string)
		}
	}
	creds, err := google.NewCredentials(values)
	if err != nil {
		return nil, handleInvalidFieldError(err)
	}
	return creds, nil
}

// handleInvalidFieldError converts a google.InvalidConfigValue into a new
// error, translating a {provider/gce/google}.OSEnvVar* value into a
// GCE config key in the new error.
func handleInvalidFieldError(err error) error {
	vErr, ok := errors.Cause(err).(*google.InvalidConfigValueError)
	if !ok {
		return err
	}
	vErr.Key = osEnvFields[vErr.Key]
	if vErr.Value == "" {
		return errors.Errorf("%s: must not be empty", vErr.Key)
	}
	return err
}
