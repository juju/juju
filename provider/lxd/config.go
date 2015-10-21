// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/container/lxd/lxdclient"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
)

// The LXD-specific config keys.
const (
	cfgNamespace  = "namespace"
	cfgRemote     = "remote"
	cfgClientCert = "client_cert"
	cfgClientKey  = "client_key"
)

// TODO(ericsnow) Use configSchema.ExampleYAML (once it is implemented)
// to generate boilerplaceConfig.

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
var boilerplateConfig = `
lxd:
    type: lxd

    # namespace identifies the namespace to associate with containers
    # created by the provider.  It is prepended to the container names.
    # By default the environment's name is used as the namespace.
    #
    # namespace: lxd
	#
	# client_cert:
	# client_key:

    # remote Identifies the LXD API server to use for managing
    # containers, if any.
    #
    # remote:

`[1:]

// configSchema defines the schema for the configuration attributes
// defined by the LXD provider.
var configSchema = environschema.Fields{
	cfgNamespace: {
		Description: `Identifies the namespace to associate with containers created by the provider.  It is prepended to the container names.  By default the environment's name is used as the namespace.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgRemote: {
		Description: `Identifies the LXD API server to use for managing containers, if any.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	cfgClientKey: {
		Description: `The client key used for connecting to a LXD host machine.`,
		Immutable:   false,
	},
	cfgClientCert: {
		Description: `The client cert used for connecting to a LXD host machine.`,
		Immutable:   false,
	},
}

var (
	// TODO(ericsnow) Extract the defaults from configSchema as soon as
	// (or if) environschema.Attr supports defaults.

	configBaseDefaults = schema.Defaults{
		cfgNamespace:  "",
		cfgRemote:     "",
		cfgClientCert: "",
		cfgClientKey:  "",
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

	configSecretFields = []string{}
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

	// The container type would be pulled from cfg if there were more
	// than one possible type for this provider.
	//cType := instance.LXD

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

	// Do final (more complex, provider-specific) validation.
	if err := ecfg.validate(); err != nil {
		return nil, errors.Trace(err)
	}

	return ecfg, nil
}

func (c *environConfig) containerType() instance.ContainerType {
	// The container type would be pulled from c.attrs if there were
	// more than one possible type for this provider.
	return instance.LXD
}

func (c *environConfig) namespace() string {
	raw := c.attrs[cfgNamespace]
	return raw.(string)
}

func (c *environConfig) remote() string {
	raw := c.attrs[cfgRemote]
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

// clientConfig builds a LXD Config based on the env config and returns it.
func (c *environConfig) clientConfig() lxdclient.Config {
	return lxdclient.Config{
		Namespace: c.namespace(),
		Remote:    c.remote(),
		// TODO(ericsnow) Also set certs...
		ClientCert: c.clientCert(),
		ClientKey:  c.clientKey(),
	}
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

	// Check sanity of complex provider-specific fields.
	if err := c.clientConfig().Validate(); err != nil {
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
	c.Config = cfg
	c.attrs = cfg.UnknownAttrs()
	return nil
}

// setRemoteFromHost sets the "remote" option to the address of the
// host machine, as reachable by an LXD container. It also ensures that
// the host's LXD is configured properly.
func (c *environConfig) setRemoteFromHost() (*environConfig, error) {
	updated, err := newValidConfig(c.Config, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Do the work.

	return updated, nil
}
