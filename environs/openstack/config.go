package openstack

import (
	"fmt"
	"launchpad.net/goose/identity"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"net/url"
)

const (
	// TODO (wallyworld) - ideally, something like http://cloud-images.ubuntu.com would have images we could use
	// but until it does, we allow a default image id to be specified.
	// This is an existing image on Canonistack - smoser-cloud-images/ubuntu-quantal-12.10-i386-server-20121017
	defaultImageId = "0f602ea9-c09e-440c-9e29-cfae5635afa3"
)

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"username":          schema.String(),
		"password":          schema.String(),
		"tenant-name":       schema.String(),
		"auth-url":          schema.String(),
		"auth-method":       schema.String(),
		"region":            schema.String(),
		"control-bucket":    schema.String(),
		"public-bucket":     schema.String(),
		"public-bucket-url": schema.String(),
		"default-image-id":  schema.String(),
	},
	schema.Defaults{
		"username":          "",
		"password":          "",
		"tenant-name":       "",
		"auth-url":          "",
		"auth-method":       string(AuthUserPass),
		"region":            "",
		"control-bucket":    "",
		"public-bucket":     "juju-dist",
		"public-bucket-url": "",
		"default-image-id":  defaultImageId,
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

func (c *environConfig) authMethod() string {
	return c.attrs["auth-method"].(string)
}

func (c *environConfig) controlBucket() string {
	return c.attrs["control-bucket"].(string)
}

func (c *environConfig) publicBucket() string {
	return c.attrs["public-bucket"].(string)
}

func (c *environConfig) publicBucketURL() string {
	return c.attrs["public-bucket-url"].(string)
}

func (c *environConfig) defaultImageId() string {
	return c.attrs["default-image-id"].(string)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

type AuthMethod string

const (
	AuthLegacy   AuthMethod = "legacy"
	AuthUserPass AuthMethod = "userpass"
)

func (p environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	v, err := configChecker.Coerce(cfg.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	ecfg := &environConfig{cfg, v.(map[string]interface{})}

	authMethod := ecfg.authMethod()
	switch AuthMethod(authMethod) {
	case AuthLegacy:
	case AuthUserPass:
	default:
		return nil, fmt.Errorf("invalid authorization method: %q", authMethod)
	}

	if ecfg.authURL() != "" {
		parts, err := url.Parse(ecfg.authURL())
		if err != nil || parts.Host == "" || parts.Scheme == "" {
			return nil, fmt.Errorf("invalid auth-url value %q", ecfg.authURL())
		}
	}
	cred := identity.CredentialsFromEnv()
	format := "required environment variable not set for credentials attribute: %s"
	if ecfg.username() == "" {
		if cred.User == "" {
			return nil, fmt.Errorf(format, "User")
		}
		ecfg.attrs["username"] = cred.User
	}
	if ecfg.password() == "" {
		if cred.Secrets == "" {
			return nil, fmt.Errorf(format, "Secrets")
		}
		ecfg.attrs["password"] = cred.Secrets
	}
	if ecfg.authURL() == "" {
		if cred.URL == "" {
			return nil, fmt.Errorf(format, "URL")
		}
		ecfg.attrs["auth-url"] = cred.URL
	}
	if ecfg.tenantName() == "" {
		if cred.TenantName == "" {
			return nil, fmt.Errorf(format, "TenantName")
		}
		ecfg.attrs["tenant-name"] = cred.TenantName
	}
	if ecfg.region() == "" {
		if cred.Region == "" {
			return nil, fmt.Errorf(format, "Region")
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
