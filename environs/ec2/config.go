package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/schema"
)

// providerConfig is a placeholder for any config information
// that we will have in a configuration file.
type providerConfig struct {
	*config.Config
	name         string
	region       string
	auth         aws.Auth
	bucket       string
	publicBucket string
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

func newConfig(config *config.Config) (*providerConfig, error) {
	v, err := configChecker.Coerce(config.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	m := v.(map[string]interface{})
	c := &providerConfig{Config: config}
	c.bucket = m["control-bucket"].(string)
	c.publicBucket = m["public-bucket"].(string)
	c.auth.AccessKey = m["access-key"].(string)
	c.auth.SecretKey = m["secret-key"].(string)
	if c.auth.AccessKey == "" || c.auth.SecretKey == "" {
		if c.auth.AccessKey != "" {
			return nil, fmt.Errorf("environment has access-key but no secret-key")
		}
		if c.auth.SecretKey != "" {
			return nil, fmt.Errorf("environment has secret-key but no access-key")
		}
		c.auth, err = aws.EnvAuth()
		if err != nil {
			return nil, err
		}
	}

	c.region = m["region"].(string)
	if _, ok := aws.Regions[c.region]; !ok {
		return nil, fmt.Errorf("invalid region name %q", c.region)
	}
	return c, nil
}
