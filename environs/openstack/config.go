package openstack

import (
	"fmt"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"os"
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
	if ecfg.username() == "" || ecfg.password() == "" || ecfg.tenantName() == "" || ecfg.authURL() == "" {
		// TODO(dimitern): get goose client to handle this
		auth, ok := getEnvAuth()
		if !ok {
			return nil, fmt.Errorf("OpenStack environment has no username, password, tenant-name, or auth-url")
		}
		ecfg.attrs["username"] = auth.username
		ecfg.attrs["password"] = auth.password
		ecfg.attrs["tenant-name"] = auth.tenantName
		ecfg.attrs["auth-url"] = auth.authURL
	}
	// We cannot validate the region name, since each OS installation
	// can have its own region names - only after authentication the
	// region names are known (from the service endpoints)

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

// TODO(dimitern): temporarily here, until goose client handles this
type envAuth struct {
	username, password, tenantName, authURL string
}

func getEnvAuth() (auth envAuth, ok bool) {
	auth = envAuth{
		username:   os.Getenv("OS_USERNAME"),
		password:   os.Getenv("OS_PASSWORD"),
		tenantName: os.Getenv("OS_TENANT_NAME"),
		authURL:    os.Getenv("OS_AUTH_URL"),
	}
	if auth.username == "" || auth.password == "" || auth.tenantName == "" || auth.authURL == "" {
		return auth, false
	}
	return auth, true
}
