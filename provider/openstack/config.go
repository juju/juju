// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"

	"github.com/juju/schema"
	"github.com/juju/utils"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	"use-floating-ip": {
		Description: "Whether a floating IP address is required to give the nodes a public IP address. Some installations assign public IP addresses by default without requiring a floating IP address.",
		Type:        environschema.Tbool,
	},
	"use-default-secgroup": {
		Description: `Whether new machine instances should have the "default" Openstack security group assigned in addition to juju defined security groups.`,
		Type:        environschema.Tbool,
	},
	"network": {
		Description: "The network label or UUID to bring machines up on when multiple networks exist.",
		Type:        environschema.Tstring,
	},
	"external-network": {
		Description: "The network label or UUID to create floating IP addresses on when multiple external networks exist.",
		Type:        environschema.Tstring,
	},
	"use-openstack-gbp": {
		Description: "Whether to use Neutrons Group-Based Policy",
		Type:        environschema.Tbool,
	},
	"policy-target-group": {
		Description: "The UUID of Policy Target Group to use for Policy Targets created.",
		Type:        environschema.Tstring,
	},
}

var configDefaults = schema.Defaults{
	"use-floating-ip":      false,
	"use-default-secgroup": false,
	"network":              "",
	"external-network":     "",
	"use-openstack-gbp":    false,
	"policy-target-group":  "",
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) useFloatingIP() bool {
	return c.attrs["use-floating-ip"].(bool)
}

func (c *environConfig) useDefaultSecurityGroup() bool {
	return c.attrs["use-default-secgroup"].(bool)
}

func (c *environConfig) network() string {
	return c.attrs["network"].(string)
}

func (c *environConfig) externalNetwork() string {
	return c.attrs["external-network"].(string)
}

func (c *environConfig) useOpenstackGBP() bool {
	return c.attrs["use-openstack-gbp"].(bool)
}

func (c *environConfig) policyTargetGroup() string {
	return c.attrs["policy-target-group"].(string)
}

type AuthMode string

const (
	AuthKeyPair  AuthMode = "keypair"
	AuthLegacy   AuthMode = "legacy"
	AuthUserPass AuthMode = "userpass"
)

// Schema returns the configuration schema for an environment.
func (EnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p EnvironProvider) ConfigSchema() schema.Fields {
	return configFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p EnvironProvider) ConfigDefaults() schema.Defaults {
	return configDefaults
}

func (p EnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}

	validated, err := cfg.ValidateUnknownAttrs(configFields, p.Configurator.GetConfigDefaults())
	if err != nil {
		return nil, err
	}
	ecfg := &environConfig{cfg, validated}

	cfgAttrs := cfg.AllAttrs()
	// If we have use-openstack-gbp set to Yes we require a proper UUID for policy-target-group.
	hasPTG := false
	if ptg := cfgAttrs["policy-target-group"]; ptg != nil && ptg.(string) != "" {
		if utils.IsValidUUIDString(ptg.(string)) {
			hasPTG = true
		} else {
			return nil, fmt.Errorf("policy-target-group has invalid UUID: %q", ptg)
		}
	}
	if useGBP := cfgAttrs["use-openstack-gbp"]; useGBP != nil && useGBP.(bool) == true {
		if hasPTG == false {
			return nil, fmt.Errorf("policy-target-group must be set when use-openstack-gbp is set")
		}
		if network := cfgAttrs["network"]; network != nil && network.(string) != "" {
			return nil, fmt.Errorf("cannot use 'network' config setting when use-openstack-gbp is set")
		}
	}

	// Check for deprecated fields and log a warning. We also print to stderr to ensure the user sees the message
	// even if they are not running with --debug.
	if defaultImageId := cfgAttrs["default-image-id"]; defaultImageId != nil && defaultImageId.(string) != "" {
		msg := fmt.Sprintf(
			"Config attribute %q (%v) is deprecated and ignored.\n"+
				"Your cloud provider should have set up image metadata to provide the correct image id\n"+
				"for your chosen series and architecture. If this is a private Openstack deployment without\n"+
				"existing image metadata, please run 'juju-metadata help' to see how suitable image"+
				"metadata can be generated.",
			"default-image-id", defaultImageId)
		logger.Warningf(msg)
	}
	if defaultInstanceType := cfgAttrs["default-instance-type"]; defaultInstanceType != nil && defaultInstanceType.(string) != "" {
		msg := fmt.Sprintf(
			"Config attribute %q (%v) is deprecated and ignored.\n"+
				"The correct instance flavor is determined using constraints, globally specified\n"+
				"when an model is bootstrapped, or individually when a charm is deployed.\n"+
				"See 'juju help bootstrap' or 'juju help deploy'.",
			"default-instance-type", defaultInstanceType)
		logger.Warningf(msg)
	}
	// Construct a new config with the deprecated attributes removed.
	for _, attr := range []string{"default-image-id", "default-instance-type"} {
		delete(cfgAttrs, attr)
		delete(ecfg.attrs, attr)
	}
	for k, v := range ecfg.attrs {
		cfgAttrs[k] = v
	}
	return config.New(config.NoDefaults, cfgAttrs)
}
