// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/gce/gceapi"
)

const (
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
	gceapi.OSEnvPrivateKey:    cfgPrivateKey,
	gceapi.OSEnvClientID:      cfgClientID,
	gceapi.OSEnvClientEmail:   cfgClientEmail,
	gceapi.OSEnvRegion:        cfgRegion,
	gceapi.OSEnvProjectID:     cfgProjectID,
	gceapi.OSEnvImageEndpoint: cfgImageEndpoint,
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

func (c *environConfig) imageEndpoint() string {
	return c.attrs[cfgImageEndpoint].(string)
}

func (c *environConfig) auth() gceapi.Auth {
	return gceapi.Auth{
		ClientID:    c.attrs[cfgClientID].(string),
		ClientEmail: c.attrs[cfgClientEmail].(string),
		PrivateKey:  []byte(c.attrs[cfgPrivateKey].(string)),
	}
}

func (c *environConfig) newConnection() *gceapi.Connection {
	return &gceapi.Connection{
		Region:    c.attrs[cfgRegion].(string),
		ProjectID: c.attrs[cfgProjectID].(string),
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
	if err := gceapi.ValidateAuth(ecfg.auth()); err != nil {
		return nil, errors.Trace(handleInvalidField(err))
	}
	if err := gceapi.ValidateConnection(ecfg.newConnection()); err != nil {
		return nil, errors.Trace(handleInvalidField(err))
	}

	// TODO(ericsnow) Follow up with someone on if it is appropriate
	// to call Apply here.
	cfg, err = ecfg.Config.Apply(ecfg.attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ecfg.Config = cfg
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
