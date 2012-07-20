package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

var configFields = schema.Fields{
	"access-key":     schema.String(),
	"secret-key":     schema.String(),
	"region":         schema.String(),
	"control-bucket": schema.String(),
	"public-bucket":  schema.String(),
}

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"access-key":     schema.String(),
		"secret-key":     schema.String(),
		"region":         schema.String(),
		"control-bucket": schema.String(),
		"public-bucket":  schema.String(),
	},
	schema.Defaults{
		"access-key":    "",
		"secret-key":    "",
		"region":        "us-east-1",
		"public-bucket": "",
	},
)

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) region() string {
	return c.attrs["region"].(string)
}

func (c *environConfig) controlBucket() string {
	return c.attrs["control-bucket"].(string)
}

func (c *environConfig) publicBucket() string {
	return c.attrs["public-bucket"].(string)
}

func (c *environConfig) accessKey() string {
	return c.attrs["access-key"].(string)
}

func (c *environConfig) secretKey() string {
	return c.attrs["secret-key"].(string)
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
	if ecfg.accessKey() == "" || ecfg.secretKey() == "" {
		auth, err := aws.EnvAuth()
		if err != nil || ecfg.accessKey() != "" || ecfg.secretKey() != "" {
			return nil, fmt.Errorf("environment has no access-key or secret-key")
		}
		ecfg.attrs["access-key"] = auth.AccessKey
		ecfg.attrs["secret-key"] = auth.SecretKey
	}
	if _, ok := aws.Regions[ecfg.region()]; !ok {
		return nil, fmt.Errorf("invalid region name %q", ecfg.region())
	}

	if old != nil {
		region, _ := old.UnknownAttrs()["region"].(string)
		if ecfg.region() != region {
			return nil, fmt.Errorf("can't change environment region from %q to %q", region, ecfg.region())
		}
	}
	return cfg.Apply(ecfg.attrs)
}
