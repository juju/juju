// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

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

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
var boilerplateConfig = `
gce:
  type: gce

  # Google Auth Info
  # The GCE provider uses OAuth to authenticate. This requires that
  # you set it up and get the relevant credentials. For more information
  # see https://cloud.google.com/compute/docs/api/how-tos/authorization.
  # Once you have the information, enter it here. All three of these are
  # required and have specific meaning to GCE.
  private-key: 
  client-email:
  client-id:

  # Google instance info
  # To provision instances and perform related operations, the provider
  # will need to know which GCE project to use and into which region to
  # provision. While the region has a default, the project ID is
  # required. For information on the project ID, see
  # https://cloud.google.com/compute/docs/projects and regarding regions
  # see https://cloud.google.com/compute/docs/zones.
  project-id:
  # region: us-central1

  # The GCE provider uses pre-built images when provisioning instances.
  # You can customize the location in which to find them with the
  # image-endpoint setting. The default value is the a location within
  # GCE, so it will give you the best speed when bootstrapping or adding
  # machines. For more information on the image cache see
  # https://cloud-images.ubuntu.com/.
  # image-endpoint: https://www.googleapis.com
`[1:]

// configFields is the spec for each GCE config value's type.
var configFields = schema.Fields{
	cfgPrivateKey:    schema.String(),
	cfgClientID:      schema.String(),
	cfgClientEmail:   schema.String(),
	cfgRegion:        schema.String(),
	cfgProjectID:     schema.String(),
	cfgImageEndpoint: schema.String(),
}

// TODO(ericsnow) Do we need custom defaults for "image-metadata-url" or
// "agent-metadata-url"? The defaults are the official ones (e.g.
// cloud-images).

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

// osEnvFields is the mapping from GCE env vars to config keys.
var osEnvFields = map[string]string{
	google.OSEnvPrivateKey:    cfgPrivateKey,
	google.OSEnvClientID:      cfgClientID,
	google.OSEnvClientEmail:   cfgClientEmail,
	google.OSEnvRegion:        cfgRegion,
	google.OSEnvProjectID:     cfgProjectID,
	google.OSEnvImageEndpoint: cfgImageEndpoint,
}

func parseOSEnv() (map[string]interface{}, error) {
	// TODO(ericsnow) Support pulling ID/PK from shell environment variables.
	return nil, nil
}

// handleInvalidField converts a config.InvalidConfigValue into a new
// error, translating a {provider/gce/google}.OSEnvVar* value into a
// GCE config key in the new error.
func handleInvalidField(err error) error {
	vErr := err.(*config.InvalidConfigValueError)
	if vErr.Reason == nil && vErr.Value == "" {
		key := osEnvFields[vErr.Key]
		return errors.Errorf("%s: must not be empty", key)
	}
	return err
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

// newConfig builds a new environConfig from the provided Config and
// returns it.
func newConfig(cfg *config.Config) *environConfig {
	return &environConfig{
		Config: cfg,
		attrs:  cfg.UnknownAttrs(),
	}
}

// prepareConfig builds a new environConfig from the provided Config and
// returns it. This includes some GCE-specific updates (including OS
// environment variables) and applying default values. The resulting
// config values are validated.
func prepareConfig(cfg *config.Config) (*environConfig, error) {

	// Make any necessary updates to the config. This needs to happen
	// before any defaults are applied.
	updates, err := parseOSEnv()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err = cfg.Apply(updates)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Finish the config.
	ecfg, err := newValidConfig(cfg, configDefaults)
	return ecfg, errors.Trace(err)
}

// newValidConfig builds a new environConfig from the provided Config
// and returns it. This includes applying the provided defaults
// values, if any. The resulting config values are validated.
func newValidConfig(cfg *config.Config, defaults map[string]interface{}) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Apply the defaults and coerce/validate the custom config attrs.
	validated, err := cfg.ValidateUnknownAttrs(configFields, defaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validCfg, err := cfg.Apply(validated)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the config.
	ecfg := newConfig(validCfg)

	// Do final validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
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

// imageEndpoint identifies where the provider should look for
// cloud images (i.e. for simplestreams).
func (c *environConfig) imageEndpoint() string {
	return c.attrs[cfgImageEndpoint].(string)
}

// auth build a new Auth based on the config and returns it.
func (c *environConfig) auth() google.Auth {
	return google.Auth{
		ClientID:    c.clientID(),
		ClientEmail: c.clientEmail(),
		PrivateKey:  []byte(c.privateKey()),
	}
}

// newConnection build a Connection based on the config and returns it.
// The resulting connection must still have its Connect called.
func (c *environConfig) newConnection() *google.Connection {
	return &google.Connection{
		Region:    c.region(),
		ProjectID: c.projectID(),
	}
}

// secret gathers the "secret" config values and returns them.
func (c *environConfig) secret() map[string]string {
	secretAttrs := make(map[string]string, len(configSecretFields))
	for _, key := range configSecretFields {
		secretAttrs[key] = c.attrs[key].(string)
	}
	return secretAttrs
}

// validate checks GCE-specific config values.
func (c environConfig) validate() error {
	// All fields must be populated, even with just the default.
	for field := range configFields {
		if c.attrs[field].(string) == "" {
			return errors.Errorf("%s: must not be empty", field)
		}
	}

	// Check sanity of GCE fields.
	if err := google.ValidateAuth(c.auth()); err != nil {
		return errors.Trace(handleInvalidField(err))
	}
	if err := google.ValidateConnection(c.newConnection()); err != nil {
		return errors.Trace(handleInvalidField(err))
	}

	return nil
}

// update applies changes from the provided config to the env config.
// Changes to any immutable attributes result in an error.
func (c *environConfig) update(cfg *config.Config) error {
	// Validate the updates. newValidConfig does not modify the "known"
	// config attributes so it is safe to call Validate here first.
	if err := config.Validate(cfg, c.Config); err != nil {
		return errors.Trace(err)
	}

	updates, err := newValidConfig(cfg, configDefaults)
	if err != nil {
		return errors.Trace(err)
	}

	// Check that no immutable fields have changed.
	attrs := updates.UnknownAttrs()
	for _, field := range configImmutableFields {
		if attrs[field] != c.attrs[field] {
			return errors.Errorf("%s: cannot change from %v to %v", field, c.attrs[field], attrs[field])
		}
	}

	// Apply the updates.
	c.Config = cfg
	c.attrs = cfg.UnknownAttrs()
	return nil
}
