package openstack

import (
	"fmt"
	"launchpad.net/goose/identity"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"net/url"
)

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"username":       schema.String(),
		"password":       schema.String(),
		"tenant-name":    schema.String(),
		"auth-url":       schema.String(),
		"region":         schema.String(),
		"control-bucket": schema.String(),
	},
	schema.Defaults{
		"username":       "",
		"password":       "",
		"tenant-name":    "",
		"auth-url":       "",
		"region":         "",
		"control-bucket": "",
	},
)

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

func (c *environConfig) controlBucket() string {
	return c.attrs["control-bucket"].(string)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	v, err := configChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	ecfg := &environConfig{cfg, v.(map[string]interface{})}

	if ecfg.authURL() != "" {
		parts, err := url.Parse(ecfg.authURL())
		if err != nil || parts.Host == "" || parts.Scheme == "" {
			return nil, fmt.Errorf("invalid auth-url value %q", ecfg.authURL())
		}
	}
	cred := identity.CredentialsFromEnv()
	missingAttributef := "required environment variable not set for credentials attribute: %s"
	if ecfg.username() == "" {
		if cred.User == "" {
			return nil, fmt.Errorf(missingAttributef, "User")
		}
		ecfg.attrs["username"] = cred.User
	}
	if ecfg.password() == "" {
		if cred.Secrets == "" {
			return nil, fmt.Errorf(missingAttributef, "Secrets")
		}
		ecfg.attrs["password"] = cred.Secrets
	}
	if ecfg.authURL() == "" {
		if cred.URL == "" {
			return nil, fmt.Errorf(missingAttributef, "URL")
		}
		ecfg.attrs["auth-url"] = cred.URL
	}
	if ecfg.tenantName() == "" {
		if cred.TenantName == "" {
			return nil, fmt.Errorf(missingAttributef, "TenantName")
		}
		ecfg.attrs["tenant-name"] = cred.TenantName
	}
	if ecfg.region() == "" {
		if cred.Region == "" {
			return nil, fmt.Errorf(missingAttributef, "Region")
		}
		ecfg.attrs["region"] = cred.Region
	}

	if old != nil {
		attrs := old.UnknownAttrs()
		if region, _ := attrs["region"].(string); ecfg.region() != region {
			return nil, fmt.Errorf("cannot change region from %q to %q", region, ecfg.region())
		}
		if controlBucket, _ := attrs["control-bucket"].(string); ecfg.controlBucket() != controlBucket {
			return nil, fmt.Errorf("cannot change control-bucket from %q to %q", controlBucket, ecfg.controlBucket())
		}
	}

	switch cfg.FirewallMode() {
	case config.FwDefault:
		ecfg.attrs["firewall-mode"] = config.FwInstance
	case config.FwInstance, config.FwGlobal:
	default:
		return nil, fmt.Errorf("unsupported firewall mode: %q", cfg.FirewallMode())
	}

	return cfg.Apply(ecfg.attrs)
}
