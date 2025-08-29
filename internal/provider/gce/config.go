// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/gce/internal/google"
)

const (
	cfgBaseImagePathKey = "base-image-path"
	vpcIDKey            = "vpc-id"
	vpcIDForceKey       = "vpc-id-force"
)

var configSchema = environschema.Fields{
	cfgBaseImagePathKey: {
		Description: "Base path to look for machine disk images.",
		Type:        environschema.Tstring,
	},
	vpcIDKey: {
		Description: "Use a specific VPC network (optional). When not specified, Juju requires a default VPC to be available for the account.",
		Type:        environschema.Tstring,
		Immutable:   true,
	},
	vpcIDForceKey: {
		Description: "Force Juju to use the GCE VPC ID specified with vpc-id, when it fails the minimum validation criteria.",
		Type:        environschema.Tbool,
		Immutable:   true,
	},
}

// configFields is the spec for each GCE config value's type.
var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configImmutableFields = []string{
	vpcIDKey,
	vpcIDForceKey,
}

var configDefaults = schema.Defaults{
	cfgBaseImagePathKey: schema.Omit,
	vpcIDKey:            google.NetworkDefaultName,
	vpcIDForceKey:       false,
}

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

	ecfg := &environConfig{cfg, attrs}
	explicitVpc, ok := attrs[vpcIDKey]
	if !ok || explicitVpc.(string) == "" && ecfg.forceVPCID() {
		return nil, fmt.Errorf("cannot use vpc-id-force without specifying vpc-id as well")
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

	return ecfg, nil
}

func (c *environConfig) baseImagePath() (string, bool) {
	path, ok := c.attrs[cfgBaseImagePathKey].(string)
	return path, ok
}

func (c *environConfig) vpcID() (string, bool) {
	vpcID, ok := c.attrs[vpcIDKey].(string)
	return vpcID, ok
}

func (c *environConfig) forceVPCID() bool {
	force, _ := c.attrs[vpcIDForceKey].(bool)
	return force
}
