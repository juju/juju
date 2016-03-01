// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/tools/lxdclient"
)

// TODO(ericsnow) Support providing cert/key file.

// The LXD-specific config keys.
const (
	cfgNamespace     = "namespace"
	cfgRemoteURL     = "remote-url"
	cfgClientCert    = "client-cert"
	cfgClientKey     = "client-key"
	cfgServerPEMCert = "server-cert"
)

// configSchema defines the schema for the configuration attributes
// defined by the LXD provider.
var configSchema = environschema.Fields{
	cfgNamespace: {
		Description: `Identifies the namespace to associate with containers created by the provider.  It is prepended to the container names.  By default the model's name is used as the namespace.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgRemoteURL: {
		Description: `Identifies the LXD API server to use for managing containers, if any.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgClientKey: {
		Description: `The client key used for connecting to a LXD host machine.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgClientCert: {
		Description: `The client cert used for connecting to a LXD host machine.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgServerPEMCert: {
		Description: `The certificate of the LXD server on the host machine.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
}

var (
	// TODO(ericsnow) Extract the defaults from configSchema as soon as
	// (or if) environschema.Attr supports defaults.

	configBaseDefaults = schema.Defaults{
		cfgNamespace:     "",
		cfgRemoteURL:     "",
		cfgClientCert:    "",
		cfgClientKey:     "",
		cfgServerPEMCert: "",
	}

	configFields, configDefaults = func() (schema.Fields, schema.Defaults) {
		fields, defaults, err := configSchema.ValidationSchema()
		if err != nil {
			panic(err)
		}
		defaults = updateDefaults(defaults, configBaseDefaults)
		return fields, defaults
	}()

	configImmutableFields = func() []string {
		var names []string
		for name, attr := range configSchema {
			if attr.Immutable {
				names = append(names, name)
			}
		}
		return names
	}()

	configSecretFields = []string{
		cfgClientKey, // only privileged agents should get to talk to LXD
	}
)

func updateDefaults(defaults schema.Defaults, updates schema.Defaults) schema.Defaults {
	updated := schema.Defaults{}
	for k, v := range defaults {
		updated[k] = v
	}
	for k, v := range updates {
		// TODO(ericsnow) Delete the item if v is nil?
		updated[k] = v
	}
	return updated
}

func adjustDefaults(cfg *config.Config, defaults map[string]interface{}) (map[string]interface{}, []string) {
	var unset []string
	updated := make(map[string]interface{})
	for k, v := range defaults {
		updated[k] = v
	}

	// Set the proper default namespace.
	raw := updated[cfgNamespace]
	if raw == nil || raw.(string) == "" {
		raw = cfg.Name()
		updated[cfgNamespace] = raw
	}

	if val, ok := cfg.UnknownAttrs()[cfgNamespace]; ok && val == "" {
		unset = append(unset, cfgNamespace)
	}

	return updated, unset
}

// TODO(ericsnow) environschema.Fields should have this...
func ensureImmutableFields(oldAttrs, newAttrs map[string]interface{}) error {
	for name, attr := range configSchema {
		if !attr.Immutable {
			continue
		}
		if newAttrs[name] != oldAttrs[name] {
			return errors.Errorf("%s: cannot change from %v to %v", name, oldAttrs[name], newAttrs[name])
		}
	}
	return nil
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

// newValidConfig builds a new environConfig from the provided Config
// and returns it. This includes applying the provided defaults
// values, if any. The resulting config values are validated.
func newValidConfig(cfg *config.Config, defaults map[string]interface{}) (*environConfig, error) {
	// Any auth credentials handling should happen first...

	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Apply the defaults and coerce/validate the custom config attrs.
	fixedDefaults, unset := adjustDefaults(cfg, defaults)
	cfg, err := cfg.Remove(unset)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, fixedDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}
	validCfg, err := cfg.Apply(validated)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Build the config.
	ecfg := newConfig(validCfg)

	// Update to defaults set via client config.
	clientCfg, err := ecfg.clientConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ecfg, err = ecfg.updateForClientConfig(clientCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Do final (more complex, provider-specific) validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
}

func (c *environConfig) namespace() string {
	raw := c.attrs[cfgNamespace]
	return raw.(string)
}

func (c *environConfig) dirname() string {
	// TODO(ericsnow) Put it under one of the juju/paths.*() directories.
	return ""
}

func (c *environConfig) remoteURL() string {
	raw := c.attrs[cfgRemoteURL]
	return raw.(string)
}

func (c *environConfig) clientCert() string {
	raw := c.attrs[cfgClientCert]
	return raw.(string)
}

func (c *environConfig) clientKey() string {
	raw := c.attrs[cfgClientKey]
	return raw.(string)
}

func (c *environConfig) serverPEMCert() string {
	raw := c.attrs[cfgServerPEMCert]
	return raw.(string)
}

// clientConfig builds a LXD Config based on the env config and returns it.
func (c *environConfig) clientConfig() (lxdclient.Config, error) {
	remote := lxdclient.Remote{
		Name:          "juju-remote",
		Host:          c.remoteURL(),
		ServerPEMCert: c.serverPEMCert(),
	}
	if c.clientCert() != "" {
		certPEM := []byte(c.clientCert())
		keyPEM := []byte(c.clientKey())
		cert := lxdclient.NewCert(certPEM, keyPEM)
		cert.Name = fmt.Sprintf("juju cert for env %q", c.Name())
		remote.Cert = &cert
	}

	cfg := lxdclient.Config{
		Namespace: c.namespace(),
		Remote:    remote,
	}
	cfg, err := cfg.WithDefaults()
	if err != nil {
		return cfg, errors.Trace(err)
	}
	return cfg, nil
}

// TODO(ericsnow) Switch to a DI testing approach and eliminiate this var.
var asNonLocal = lxdclient.Config.UsingTCPRemote

func (c *environConfig) updateForClientConfig(clientCfg lxdclient.Config) (*environConfig, error) {
	nonlocal, err := asNonLocal(clientCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	clientCfg = nonlocal

	c.attrs[cfgNamespace] = clientCfg.Namespace

	c.attrs[cfgRemoteURL] = clientCfg.Remote.Host

	c.attrs[cfgServerPEMCert] = clientCfg.Remote.ServerPEMCert

	var cert lxdclient.Cert
	if clientCfg.Remote.Cert != nil {
		cert = *clientCfg.Remote.Cert
	}
	c.attrs[cfgClientCert] = string(cert.CertPEM)
	c.attrs[cfgClientKey] = string(cert.KeyPEM)

	// Apply the updates.
	cfg, err := c.Config.Apply(c.attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newConfig(cfg), nil
}

// secret gathers the "secret" config values and returns them.
func (c *environConfig) secret() map[string]string {
	if len(configSecretFields) == 0 {
		return nil
	}

	secretAttrs := make(map[string]string, len(configSecretFields))
	for _, key := range configSecretFields {
		secretAttrs[key] = c.attrs[key].(string)
	}
	return secretAttrs
}

// validate checks more complex LCD-specific config values.
func (c environConfig) validate() error {
	// All fields must be populated, even with just the default.
	// TODO(ericsnow) Shouldn't configSchema support this?
	for field := range configFields {
		if dflt, ok := configDefaults[field]; ok && dflt == "" {
			continue
		}
		if c.attrs[field].(string) == "" {
			return errors.Errorf("%s: must not be empty", field)
		}
	}

	// If cert is provided then key must be (and vice versa).
	if c.clientCert() == "" && c.clientKey() != "" {
		return errors.Errorf("missing %s (got %s value %q)", cfgClientCert, cfgClientKey, c.clientKey())
	}
	if c.clientCert() != "" && c.clientKey() == "" {
		return errors.Errorf("missing %s (got %s value %q)", cfgClientKey, cfgClientCert, c.clientCert())
	}

	// Check sanity of complex provider-specific fields.
	cfg, err := c.clientConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if err := cfg.Validate(); err != nil {
		return errors.Trace(err)
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
	if err := ensureImmutableFields(c.attrs, attrs); err != nil {
		return errors.Trace(err)
	}

	// Apply the updates.
	// TODO(ericsnow) Should updates.Config be set in instead of cfg?
	c.Config = cfg
	c.attrs = cfg.UnknownAttrs()
	return nil
}
