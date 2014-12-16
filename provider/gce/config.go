// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

// TODO(ericsnow) This file needs a once-over to verify correctness.

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
)

const (
	// These are not GCE-official environment variable names.
	osEnvPrivateKey    = "GCE_PRIVATE_KEY"
	osEnvClientID      = "GCE_CLIENT_ID"
	osEnvClientEmail   = "GCE_CLIENT_EMAIL"
	osEnvRegion        = "GCE_REGION"
	osEnvProjectID     = "GCE_PROJECT_ID"
	osEnvImageEndpoint = "GCE_IMAGE_URL"

	cfgPrivateKey    = "private-key"
	cfgClientID      = "client-id"
	cfgClientEmail   = "client-email"
	cfgRegion        = "region"
	cfgProjectID     = "project-id"
	cfgImageEndpoint = "image-endpoint"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
var boilerplateConfig = `
gce:
  type: gce

  # Google Auth Info
  private-key: 
  client-email:
  client-id:

  # Google instance info
  # region: us-central1
  project-id:
  # image-endpoint: https://www.googleapis.com
`[1:]

var osEnvFields = map[string]string{
	osEnvPrivateKey:    cfgPrivateKey,
	osEnvClientID:      cfgClientID,
	osEnvClientEmail:   cfgClientEmail,
	osEnvRegion:        cfgRegion,
	osEnvProjectID:     cfgProjectID,
	osEnvImageEndpoint: cfgImageEndpoint,
}

var configFields = schema.Fields{
	cfgPrivateKey:    schema.String(),
	cfgClientID:      schema.String(),
	cfgClientEmail:   schema.String(),
	cfgRegion:        schema.String(),
	cfgProjectID:     schema.String(),
	cfgImageEndpoint: schema.String(),
}

var configDefaults = schema.Defaults{
	// TODO(ericsnow) Do we really want a default region (see azure)?
	cfgRegion: "us-central1",
	// See http://cloud-images.ubuntu.com/releases/streams/v1/com.ubuntu.cloud:released:gce.json
	cfgImageEndpoint: "https://www.googleapis.com",
}

var configSecretFields = []string{
	cfgPrivateKey,
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
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) privateKey() string {
	return c.attrs[cfgPrivateKey].(string)
}

func (c *environConfig) clientID() string {
	return c.attrs[cfgClientID].(string)
}

func (c *environConfig) clientEmail() string {
	return c.attrs[cfgClientEmail].(string)
}

func (c *environConfig) region() string {
	return c.attrs[cfgRegion].(string)
}

func (c *environConfig) projectID() string {
	return c.attrs[cfgProjectID].(string)
}

func (c *environConfig) imageURL() string {
	return c.attrs[cfgImageEndpoint].(string)
}

func (c *environConfig) auth() gceAuth {
	return gceAuth{
		clientID:    c.attrs[cfgClientID].(string),
		clientEmail: c.attrs[cfgClientEmail].(string),
		privateKey:  []byte(c.attrs[cfgPrivateKey].(string)),
	}
}

func (c *environConfig) newConnection() *gceConnection {
	return &gceConnection{
		region:    c.attrs[cfgRegion].(string),
		projectID: c.attrs[cfgProjectID].(string),
	}
}

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes and coerce the values (base config first
	// then custom).
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ecfg := &environConfig{cfg, validated}

	// TODO(ericsnow) Support pulling ID/PK from shell environment variables.

	// Check that no immutable fields have changed.
	if old != nil {
		attrs := old.UnknownAttrs()
		for _, field := range configImmutableFields {
			if attrs[field] != validated[field] {
				return nil, errors.Errorf("%s: cannot change from %v to %v", field, attrs[field], validated[field])
			}
		}
	}

	// Check sanity of GCE fields.
	if err := ecfg.auth().validate(); err != nil {
		return nil, errors.Trace(handleInvalidField(err))
	}
	if err := ecfg.newConnection().validate(); err != nil {
		return nil, errors.Trace(handleInvalidField(err))
	}

	// Check image URL value.
	// TODO(ericsnow) Implement the check.

	return ecfg, nil
}

func handleInvalidField(err error) error {
	vErr := err.(*config.InvalidConfigValue)
	if vErr.Reason == nil && vErr.Value == "" {
		key := osEnvFields[vErr.Key]
		return errors.Errorf("%s: must not be empty", key)
	}
	return err
}
