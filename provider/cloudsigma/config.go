// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/altoros/gosigma"
	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/config"
)

// boilerplateConfig will be shown in help output, so please keep it up to
// date when you change environment configuration below.
var boilerplateConfig = `# https://juju.ubuntu.com/docs/config-cloudsigma.html
cloudsigma:
    type: cloudsigma

    # region holds the cloudsigma region (zrh, lvs, ...).
    #
    # region: <your region>

    # credentials for CloudSigma account
    #
    # username: <your username>
    # password: <secret>
`

const (
	defaultStoragePort = 8040
)

var configFields = schema.Fields{
	"username": schema.String(),
	"password": schema.String(),
	"region":   schema.String(),
}

var configDefaultFields = schema.Defaults{
	"username": "",
	"password": "",
	"region":   gosigma.DefaultRegion,
}

var configSecretFields = []string{
	"password",
}

var configImmutableFields = []string{
	"region",
}

func prepareConfig(cfg *config.Config) (*config.Config, error) {
	// Turn an incomplete config into a valid one, if possible.
	attrs := cfg.AllAttrs()

	if _, ok := attrs["uuid"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, errors.Trace(err)
		}
		attrs["uuid"] = uuid.String()
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
		return nil, errors.Trace(err)
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
		return nil, errors.Trace(err)
	}
	for field := range configFields {
		if newAttrs[field] == "" {
			return nil, errors.Errorf("%s: must not be empty", field)
		}
	}

	// If an old config was supplied, check any immutable fields have not changed.
	if old != nil {
		for _, field := range configImmutableFields {
			if old.attrs[field] != newAttrs[field] {
				return nil, errors.Errorf(
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
		return nil, errors.Trace(err)
	}
	ecfg := &environConfig{
		Config: newCfg,
		attrs:  newAttrs,
	}

	return ecfg, nil
}

// configChanged checks if CloudSigma client environment configuration is changed
func (c environConfig) clientConfigChanged(newConfig *environConfig) bool {
	// compare
	if newConfig.region() != c.region() || newConfig.username() != c.username() ||
		newConfig.password() != c.password() {
		return true
	}

	return false
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c environConfig) region() string {
	return c.attrs["region"].(string)
}

func (c environConfig) username() string {
	return c.attrs["username"].(string)
}

func (c environConfig) password() string {
	return c.attrs["password"].(string)
}
