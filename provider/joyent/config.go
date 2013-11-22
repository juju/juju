// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"launchpad.net/juju-core/utils"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
const boilerplateConfig = `joyent:
  type: joyent

  # SDC config
  # Can be set via env variables, or specified here
  # sdc-user: <secret>
  # Can be set via env variables, or specified here
  # sdc-key-id: <secret>
  # region defaults to us-west-1, override if required
  # sdc-region: us-west-1

  # Manta config
  # Can be set via env variables, or specified here
  # manta-user: <secret>
  # Can be set via env variables, or specified here
  # manta-key-id: <secret>
  # region defaults to us-east, override if required
  # manta-region: us-east
`

const (
	SdcAccount = "SDC_ACCOUNT"
	SdcKeyId   = "SDC_KEY_ID"
	SdcUrl     = "SDC_URL"
	MantaUser  = "MANTA_USER"
	MantaKeyId = "MANTA_KEY_ID"
	MantaUrl   = "MANTA_URL"
)

var environmentVariables = map[string]string{
	"sdc-user":     SdcAccount,
	"sdc-key-id":   SdcKeyId,
	"manta-user":   MantaUser,
	"manta-key-id": MantaKeyId,
}

var configFields = schema.Fields{
	"sdc-user":     schema.String(),
	"sdc-key-id":   schema.String(),
	"sdc-region":   schema.String(),
	"manta-user":   schema.String(),
	"manta-key-id": schema.String(),
	"manta-region": schema.String(),
	"control-dir":  schema.String(),
}

var configDefaultFields = schema.Defaults{
	"sdc-region":   "us-west-1",
	"manta-region": "us-east",
}

var configSecretFields = []string{
	"sdc-user",
	"sdc-key-id",
	"manta-user",
	"manta-key-id",
}

var configImmutableFields = []string{
	"sdc-region",
	"manta-region",
}

func prepareConfig(cfg *config.Config) (*config.Config, error) {
	// Turn an incomplete config into a valid one, if possible.
	attrs := cfg.UnknownAttrs()

	if _, ok := attrs["control-dir"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		attrs["control-bucket"] = fmt.Sprintf("%x", uuid.Raw())
	}

	// Read env variables
	for _, field := range configSecretFields {
		// If field is not set, get it from env variables
		if attrs[field] == "" {
			localEnvVariable := os.Getenv(environmentVariables[field])
			if localEnvVariable != "" {
				attrs[field] = localEnvVariable
			} else {
				return nil, fmt.Errorf("cannot get %s value from environment variables %s", field, environmentVariables[field])
			}
		}
	}

	return cfg.Apply(attrs)
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
	// In particular, if you want to extract (say) credentials from the user's
	// shell environment variables, you'll need to allow missing values to pass
	// through the schema by setting a value of schema.Omit in the configFields
	// map, and then to set and check them at this point. These values *must* be
	// stored in newAttrs: a Config will be generated on the user's machine only
	// to begin with, and will subsequently be used on a different machine that
	// will probably not have those variables set.
	newAttrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaultFields)
	if err != nil {
		return nil, err
	}
	for field := range configFields {
		if newAttrs[field] == "" {
			return nil, fmt.Errorf("%s: must not be empty", field)
		}
	}

	// If an old config was supplied, check any immutable fields have not changed.
	if old != nil {
		for _, field := range configImmutableFields {
			if old.attrs[field] != newAttrs[field] {
				return nil, fmt.Errorf(
					"%s: cannot change from %v to %v",
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

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (ecfg *environConfig) sdcRegion() string {
	return ecfg.attrs["sdc-region"].(string)
}

func (ecfg *environConfig) sdcUser() string {
	return ecfg.attrs["sdc-user"].(string)
}

func (ecfg *environConfig) sdcKeyId() string {
	return ecfg.attrs["sdc-key-id"].(string)
}

func (ecfg *environConfig) mantaRegion() string {
	return ecfg.attrs["manta-region"].(string)
}

func (ecfg *environConfig) mantaUser() string {
	return ecfg.attrs["manta-user"].(string)
}

func (ecfg *environConfig) mantaKeyId() string {
	return ecfg.attrs["manta-key-id"].(string)
}

func (c *environConfig) controlDir() string {
	return c.attrs["control-dir"].(string)
}
