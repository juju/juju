// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"

	"launchpad.net/gojoyent/joyent"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
const boilerplateConfig = `joyent:
  type: joyent
  admin-secret: {{rand}}

  # Can be set via the env variable SDC_ACCOUNT or MANTA_USER, or specified here
  # user: <secret>
  # Can be set via the env variable SDC_KEY_ID or MANTA_KEY_ID, or specified here
  # key-id: <secret>
  # region defaults to us-east-1, override if required
  # region: us-east-1

  # this exists to demonstrate how to deal with sensitive values.
  skeleton-secret-field: <cloud credentials, for example>

  # this exists to demonstrate how to deal with values that can't change; and
  # also how to use Prepare to fill in values that don't makes sense with
  # static defaults but can be inferred or chosen at runtime.
  # skeleton-immutable-field: <a storage bucket name, for example>

  # this exists to demonstrate how to deal with static default values that
  # some users may wish to override
  # skeleton-default-field: <specific default value>

`

/*var configFields = schema.Fields{
	"skeleton-secret-field":    schema.String(),
	"skeleton-immutable-field": schema.String(),
	"skeleton-default-field":   schema.String(),
}

var configDefaultFields = schema.Defaults{
	"skeleton-default-field": "<specific default value>",
}*/

var configFields = schema.Fields{
	"user":           		schema.String(),
	"key-id":           	schema.String(),
	"region":               schema.String(),
}

var configDefaultFields = schema.Defaults{
	"user":           		"",
	"key-id":           	"",
	"region":               "us-east-1",
}

var configSecretFields = []string{
	"skeleton-secret-field",
}

var configImmutableFields = []string{
	"skeleton-immutable-field",
}

func validateConfig(cfg *config.Config, old *environConfig) (*environConfig, error) {
	// Check sanity of juju-level fields.
	var oldCfg *config.Config
	if old != nil {
		oldCfg = old.Config
	}
	if err := config.Validate(cfg, oldCfg); err != nil {
		return nil, err
	}

	// Extract validated provider-specific fields. All of configFields will be
	// present in validated, and defaults will be inserted if necessary. If the
	// schema you passed in doesn't quite express what you need, you can make
	// whatever checks you need here, before continuing.
	newAttrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaultFields)
	if err != nil {
		return nil, err
	}
	for field := range configFields {
		if newAttrs[field] == "" {
			return nil, fmt.Errorf("invalid %q field: must not be empty", field)
		}
	}

	// If an old config was supplied, check any immutable fields have not changed.
	if old != nil {
		for _, field := range configImmutableFields {
			if old.attrs[field] != newAttrs[field] {
				return nil, fmt.Errorf(
					"invalid %q field: cannot change from %v to %v",
					field, old.attrs[field], newAttrs[field],
				)
			}
		}
	}

	// Merge the validated provider-specific fields into the original config,
	// to ensure the object we return is internally consistent.
	newCfg, err := cfg.Apply(newAttrs)
	if err != nil {
		return nil, err
	}
	return &environConfig{
		Config: newCfg,
		attrs:  newAttrs,
	}, nil
}

func prepareConfig(cfg *config.Config) (*config.Config, error) {

}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

/*func (ecfg *environConfig) secretField() string {
	return ecfg.attrs["skeleton-secret-field"].(string)
}

func (ecfg *environConfig) immutableField() string {
	return ecfg.attrs["skeleton-immutable-field"].(string)
}

func (ecfg *environConfig) defaultField() string {
	return ecfg.attrs["skeleton-default-field"].(string)
} */

func (ecfg *environConfig) region() string {
	return ecfg.attrs["region"].(string)
}

func (ecfg *environConfig) user() string {
	return ecfg.attrs["user"].(string)
}

func (ecfg *environConfig) keyId() string {
	return ecfg.attrs["key-id"].(string)
}
