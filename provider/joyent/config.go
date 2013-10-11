// Copyright 2013 Joyent Inc.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
const boilerplateConfig = `joyent:
  type: joyent

  # Can be set via env variables, or specified here
  # user: <secret>
  # Can be set via env variables, or specified here
  # key-id: <secret>
  # region defaults to us-east-1, override if required
  # region: us-east-1

`

const (
	SdcAccount        = "SDC_ACCOUNT"
	SdcKeyId          = "SDC_KEY_ID"
	MantaUser         = "MANTA_USER"
	MantaKeyId        = "MANTA_KEY_ID"
)

var environmentVariables = map[string][]string{
	"user": 	{SdcAccount, MantaUser},
	"key-id": 	{SdcKeyId, MantaKeyId},
}

var configFields = schema.Fields{
	"user":           		schema.String(),
	"key-id":           	schema.String(),
	"region":               schema.String(),
}

var configDefaultFields = schema.Defaults{
	"region":				"us-east-1",
}

var configSecretFields = []string{
	"user",
	"key-id",
}

var configImmutableFields = []string{
	"region",
}

func prepareConfig(cfg *config.Config) (*config.Config, error) {
	// Turn an incomplete config into a valid one, if possible.
	attrs := cfg.UnknownAttrs()

	// Read env variables
	for _,field := range configSecretFields {
		// If field is not set, get it from env variables
		fmt.Printf("Secret field: %s", field)
		if attrs[field] == "" {
			for _,envVariable := range environmentVariables[field] {
				fmt.Printf("-- Trying to read env variable %s", envVariable)
				localEnvVariable := os.Getenv(envVariable)
				fmt.Printf("-- Got: %s", localEnvVariable)
				if localEnvVariable != "" {
					attrs[field] = localEnvVariable
				}
			}
			if attrs[field] == "" {
				return nil, fmt.Errorf("cannot get %s value from environment variables %s or %s", field, environmentVariables[field][0], environmentVariables[field][1])
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

func (ecfg *environConfig) region() string {
	return ecfg.attrs["region"].(string)
}

func (ecfg *environConfig) user() string {
	return ecfg.attrs["user"].(string)
}

func (ecfg *environConfig) keyId() string {
	return ecfg.attrs["key-id"].(string)
}
