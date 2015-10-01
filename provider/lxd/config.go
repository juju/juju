// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/lxd/lxd_client"
)

// The LXD-specific config keys.
const (
	cfgNamespace = "namespace"
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

`[1:]

// configSchema defines the schema for the configuration attributes
// defined by the LXD provider.
var configSchema = environschema.Fields{
	cfgNamespace: {
		Description: `Identifies the namespace to associate with containers created by the provider.  It is prepended to the container names.  By default the environment's name is used as the namespace.`,
		Type:        environschema.Tstring,
		Immutable:   true,
	},
}

var (
	// TODO(ericsnow) Extract the defaults from configSchema as soon as
	// (or if) environschema.Attr supports defaults.

	configBaseDefaults = schema.Defaults{
		cfgNamespace: "",
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

func adjustDefaults(cfg *config.Config, defaults map[string]interface{}) map[string]interface{} {
	updated := make(map[string]interface{})
	for k, v := range defaults {
		updated[k] = v
	}

	// The container type would be pulled from cfg if there were more
	// than one possible type for this provider.
	//cType := instance.LXD

	// Set the proper default namespace.
	raw := defaults[cfgNamespace]
	if raw == nil || raw.(string) == "" {
		raw = cfg.Name()
		defaults[cfgNamespace] = raw
	}

	return updated
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
	defaults = adjustDefaults(cfg, defaults)
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

// clientConfig builds a LXD Config based on the env config and returns it.
func (c *environConfig) clientConfig() lxd_client.Config {
	return lxd_client.Config{
		Namespace: c.namespace(),
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
