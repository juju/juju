// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

const (
	cfgBaseImagePath = "base-image-path"
	cfgVpcId         = "vpc-id"
)

var configSchema = environschema.Fields{
	cfgBaseImagePath: {
		Description: "Base path to look for machine disk images.",
		Type:        environschema.Tstring,
	},
	cfgVpcId: {
		Description: "Use a specific VPC network name (optional). When not specified, Juju requires the default VPC network.",
		Example:     "default",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
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

var configDefaults = schema.Defaults{
	cfgBaseImagePath: schema.Omit,
	cfgVpcId:         schema.Omit,
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

	ecfg := &environConfig{
		config: cfg,
		attrs:  attrs,
	}
	if old != nil {
		attrs := old.UnknownAttrs()
		if vpcID, _ := attrs["vpc-id"].(string); vpcID != ecfg.vpcID() {
			return nil, fmt.Errorf("cannot change vpc-id from %q to %q", vpcID, ecfg.vpcID())
		}
	}

	return ecfg, nil
}

func (c *environConfig) baseImagePath() (string, bool) {
	path, ok := c.attrs[cfgBaseImagePath].(string)
	return path, ok
}

func (c *environConfig) vpcID() string {
	return c.attrs["vpc-id"].(string)
}
