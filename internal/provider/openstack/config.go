// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/schema"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/configschema"
)

const (
	ExternalNetworkKey    = "external-network"
	NetworkKey            = "network"
	PolicyTargetGroupKey  = "policy-target-group"
	UseDefaultSecgroupKey = "use-default-secgroup"
	UseOpenstackGBPKey    = "use-openstack-gbp"
)

var configSchema = configschema.Fields{
	UseDefaultSecgroupKey: {
		Description: `Whether new machine instances should have the "default" Openstack security group assigned in addition to juju defined security groups.`,
		Type:        configschema.Tbool,
	},
	NetworkKey: {
		Description: "The network label or UUID to bring machines up on when multiple networks exist.",
		Type:        configschema.Tstring,
	},
	ExternalNetworkKey: {
		Description: "The network label or UUID to create floating IP addresses on when multiple external networks exist.",
		Type:        configschema.Tstring,
	},
	UseOpenstackGBPKey: {
		Description: "Whether to use Neutrons Group-Based Policy",
		Type:        configschema.Tbool,
	},
	PolicyTargetGroupKey: {
		Description: "The UUID of Policy Target Group to use for Policy Targets created.",
		Type:        configschema.Tstring,
	},
}

var configDefaults = schema.Defaults{
	UseDefaultSecgroupKey: false,
	NetworkKey:            "",
	ExternalNetworkKey:    "",
	UseOpenstackGBPKey:    false,
	PolicyTargetGroupKey:  "",
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

func (c *environConfig) useDefaultSecurityGroup() bool {
	return c.attrs[UseDefaultSecgroupKey].(bool)
}

func (c *environConfig) networks() []string {
	raw := strings.Split(c.attrs[NetworkKey].(string), ",")
	res := make([]string, len(raw))
	for i, net := range raw {
		res[i] = strings.TrimSpace(net)
	}
	return res
}

func (c *environConfig) externalNetwork() string {
	return c.attrs[ExternalNetworkKey].(string)
}

func (c *environConfig) useOpenstackGBP() bool {
	return c.attrs[UseOpenstackGBPKey].(bool)
}

func (c *environConfig) policyTargetGroup() string {
	return c.attrs[PolicyTargetGroupKey].(string)
}

type AuthMode string

// Schema returns the configuration schema for an environment.
func (EnvironProvider) Schema() configschema.Fields {
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

// ModelConfigDefaults provides a set of default model config attributes that
// should be set on a models config if they have not been specified by the user.
func (p EnvironProvider) ModelConfigDefaults(_ context.Context) (map[string]any, error) {
	return map[string]any{
		config.StorageDefaultBlockSourceKey: CinderProviderType,
	}, nil
}

func (p EnvironProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(ctx, cfg, old); err != nil {
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
	if ptg := cfgAttrs[PolicyTargetGroupKey]; ptg != nil && ptg.(string) != "" {
		if utils.IsValidUUIDString(ptg.(string)) {
			hasPTG = true
		} else {
			return nil, fmt.Errorf("policy-target-group has invalid UUID: %q", ptg)
		}
	}
	if useGBP := cfgAttrs[UseOpenstackGBPKey]; useGBP != nil && useGBP.(bool) == true {
		if hasPTG == false {
			return nil, fmt.Errorf("policy-target-group must be set when use-openstack-gbp is set")
		}
		if network := cfgAttrs[NetworkKey]; network != nil && network.(string) != "" {
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
		logger.Warningf(ctx, msg)
	}
	if defaultInstanceType := cfgAttrs["default-instance-type"]; defaultInstanceType != nil && defaultInstanceType.(string) != "" {
		msg := fmt.Sprintf(
			"Config attribute %q (%v) is deprecated and ignored.\n"+
				"The correct instance flavor is determined using constraints, globally specified\n"+
				"when an model is bootstrapped, or individually when a charm is deployed.\n"+
				"See 'juju help bootstrap' or 'juju help deploy'.",
			"default-instance-type", defaultInstanceType)
		logger.Warningf(ctx, msg)
	}

	// Construct a new config with the ignored, deprecated attributes removed.
	for _, attr := range []string{"default-image-id", "default-instance-type"} {
		delete(cfgAttrs, attr)
		delete(ecfg.attrs, attr)
	}
	for k, v := range ecfg.attrs {
		cfgAttrs[k] = v
	}
	return config.New(config.NoDefaults, cfgAttrs)
}
