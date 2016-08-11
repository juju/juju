// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
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

var configSchema = environschema.Fields{}

// configFields is the spec for each GCE config value's type.
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configImmutableFields = []string{}

var configDefaults = schema.Defaults{}

type environConfig struct {
	config *config.Config
	attrs  map[string]interface{}
}

// newConfig builds a new environConfig from the provided Config
// filling in default values, if any. It returns an error if the
// resulting configuration is not valid.
func newConfig(cfg, old *config.Config) (*environConfig, error) {
	// Ensure that the provided config is valid.
	if err := config.Validate(cfg, old); err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if old != nil {
		// There's an old configuration. Validate it so that any
		// default values are correctly coerced for when we check
		// the old values later.
		oldEcfg, err := newConfig(old, nil)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid base config")
		}
		for _, attr := range configImmutableFields {
			oldv, newv := oldEcfg.attrs[attr], attrs[attr]
			if oldv != newv {
				return nil, errors.Errorf(
					"%s: cannot change from %v to %v",
					attr, oldv, newv,
				)
			}
		}
	}

	ecfg := &environConfig{
		config: cfg,
		attrs:  attrs,
	}
	return ecfg, nil
}
