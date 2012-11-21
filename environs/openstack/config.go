package openstack

import (
	"fmt"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
	"os"
)

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"username":     schema.String(),
		"password":     schema.String(),
		"region":       schema.String(),
		"tenant-id":    schema.String(),
		"identity-url": schema.String(),
		"container":    schema.String(),
	},
	schema.Defaults{
		"username":     "",
		"password":     "",
		"region":       "",
		"tenant-id":    "",
		"identity-url": "",
		"container":    "",
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

func (c *environConfig) tenantId() string {
	return c.attrs["tenant-id"].(string)
}

func (c *environConfig) identityURL() string {
	return c.attrs["identity-url"].(string)
}

func (c *environConfig) container() string {
	return c.attrs["container"].(string)
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
	if ecfg.username() == "" ||
		ecfg.password() == "" ||
		ecfg.tenantId() == "" ||
		ecfg.identityURL() == "" {
		// TODO: get goose client to handle this
		auth, err := dummyEnvAuth()
		if err != nil ||
			ecfg.username() != "" ||
			ecfg.password() != "" ||
			ecfg.tenantId() != "" ||
			ecfg.identityURL() != "" {
			return nil, fmt.Errorf("environment has no username, password, tenant-id, or identity-url")
		}
		ecfg.attrs["username"] = auth.username
		ecfg.attrs["password"] = auth.password
		ecfg.attrs["tenant-id"] = auth.tenantId
		ecfg.attrs["identity-url"] = auth.identityURL
	}
	// We cannot validate the region name, since each OS installation
	// can have its own region names - only after authentication the
	// region names are known (from the service endpoints)

	if old != nil {
		attrs := old.UnknownAttrs()
		if region, _ := attrs["region"].(string); ecfg.region() != region {
			return nil, fmt.Errorf("cannot change region from %q to %q", region, ecfg.region())
		}
		if container, _ := attrs["container"].(string); ecfg.container() != container {
			return nil, fmt.Errorf("cannot change container from %q to %q", container, ecfg.container())
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

// TODO: temporarily here, until goose client handles this
type dummyAuth struct {
	username, password, tenantId, identityURL string
}

func dummyEnvAuth() (dummyAuth, error) {
	return dummyAuth{
		username:    os.Getenv("OS_USERNAME"),
		password:    os.Getenv("OS_PASSWORD"),
		tenantId:    os.Getenv("OS_TENANT_NAME"),
		identityURL: os.Getenv("OS_AUTH_URL"),
	}, nil
}
