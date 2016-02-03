// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"gopkg.in/goose.v1/identity"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	"username": {
		Description: "The user name  to use when auth-mode is userpass.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvUser,
		Group:       environschema.AccountGroup,
	},
	"password": {
		Description: "The password to use when auth-mode is userpass.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvSecrets,
		Group:       environschema.AccountGroup,
		Secret:      true,
	},
	"tenant-name": {
		Description: "The openstack tenant name.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvTenantName,
		Group:       environschema.AccountGroup,
	},
	"auth-url": {
		Description: "The keystone URL for authentication.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvAuthURL,
		Example:     "https://yourkeystoneurl:443/v2.0/",
		Group:       environschema.AccountGroup,
	},
	"auth-mode": {
		Description: "The authentication mode to use. When set to keypair, the access-key and secret-key parameters should be set; when set to userpass or legacy, the username and password parameters should be set.",
		Type:        environschema.Tstring,
		Values:      []interface{}{AuthKeyPair, AuthLegacy, AuthUserPass},
		Group:       environschema.AccountGroup,
	},
	"access-key": {
		Description: "The access key to use when auth-mode is set to keypair.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvUser,
		Group:       environschema.AccountGroup,
		Secret:      true,
	},
	"secret-key": {
		Description: "The secret key to use when auth-mode is set to keypair.",
		EnvVars:     identity.CredEnvSecrets,
		Group:       environschema.AccountGroup,
		Type:        environschema.Tstring,
		Secret:      true,
	},
	"region": {
		Description: "The openstack region.",
		Type:        environschema.Tstring,
		EnvVars:     identity.CredEnvRegion,
	},
	"use-floating-ip": {
		Description: "Whether a floating IP address is required to give the nodes a public IP address. Some installations assign public IP addresses by default without requiring a floating IP address.",
		Type:        environschema.Tbool,
	},
	"use-default-secgroup": {
		Description: `Whether new machine instances should have the "default" Openstack security group assigned.`,
		Type:        environschema.Tbool,
	},
	"network": {
		Description: "The network label or UUID to bring machines up on when multiple networks exist.",
		Type:        environschema.Tstring,
	},
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

func (c *environConfig) region() string {
	return c.attrs["region"].(string)
}

func (c *environConfig) username() string {
	return c.attrs["username"].(string)
}

func (c *environConfig) password() string {
	return c.attrs["password"].(string)
}

func (c *environConfig) tenantName() string {
	return c.attrs["tenant-name"].(string)
}

func (c *environConfig) authURL() string {
	return c.attrs["auth-url"].(string)
}

func (c *environConfig) authMode() AuthMode {
	return AuthMode(c.attrs["auth-mode"].(string))
}

func (c *environConfig) accessKey() string {
	return c.attrs["access-key"].(string)
}

func (c *environConfig) secretKey() string {
	return c.attrs["secret-key"].(string)
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

func (p EnvironProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}

	validated, err := cfg.ValidateUnknownAttrs(configFields, p.Configurator.GetConfigDefaults())
	if err != nil {
		return nil, err
	}

	// Add Openstack specific defaults.
	providerDefaults := map[string]interface{}{}

	// Storage.
	if _, ok := cfg.StorageDefaultBlockSource(); !ok {
		providerDefaults[config.StorageDefaultBlockSourceKey] = CinderProviderType
	}
	if len(providerDefaults) > 0 {
		if cfg, err = cfg.Apply(providerDefaults); err != nil {
			return nil, err
		}
	}

	ecfg := &environConfig{cfg, validated}

	switch ecfg.authMode() {
	case AuthUserPass, AuthLegacy:
		if ecfg.username() == "" {
			return nil, errors.NotValidf("missing username")
		}
		if ecfg.password() == "" {
			return nil, errors.NotValidf("missing password")
		}
	case AuthKeyPair:
		if ecfg.accessKey() == "" {
			return nil, errors.NotValidf("missing access-key")
		}
		if ecfg.secretKey() == "" {
			return nil, errors.NotValidf("missing secret-key")
		}
	default:
		return nil, fmt.Errorf("unexpected authentication mode %q", ecfg.authMode())
	}

	if ecfg.authURL() == "" {
		return nil, errors.NotValidf("missing auth-url")
	}
	if ecfg.tenantName() == "" {
		return nil, errors.NotValidf("missing tenant-name")
	}
	if ecfg.region() == "" {
		return nil, errors.NotValidf("missing region")
	}

	parts, err := url.Parse(ecfg.authURL())
	if err != nil || parts.Host == "" || parts.Scheme == "" {
		return nil, fmt.Errorf("invalid auth-url value %q", ecfg.authURL())
	}

	if old != nil {
		attrs := old.UnknownAttrs()
		if region, _ := attrs["region"].(string); ecfg.region() != region {
			return nil, fmt.Errorf("cannot change region from %q to %q", region, ecfg.region())
		}
	}

	// Check for deprecated fields and log a warning. We also print to stderr to ensure the user sees the message
	// even if they are not running with --debug.
	cfgAttrs := cfg.AllAttrs()
	if defaultImageId := cfgAttrs["default-image-id"]; defaultImageId != nil && defaultImageId.(string) != "" {
		msg := fmt.Sprintf(
			"Config attribute %q (%v) is deprecated and ignored.\n"+
				"Your cloud provider should have set up image metadata to provide the correct image id\n"+
				"for your chosen series and archietcure. If this is a private Openstack deployment without\n"+
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
