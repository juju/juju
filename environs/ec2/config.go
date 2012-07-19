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
	}, []string{
		"access-key",
		"secret-key",
		"region",
		"public-bucket",
	},
)

func newConfig(config *config.Config) (*providerConfig, error) {
	v, err := configChecker.Coerce(config.UnknownAttrs(), nil)
	if err != nil {
		return nil, err
	}
	m := v.(schema.MapType)
	c := &providerConfig{Config: config}
	c.bucket = m["control-bucket"].(string)
	c.publicBucket = maybeString(m["public-bucket"], "")
	c.auth.AccessKey = maybeString(m["access-key"], "")
	c.auth.SecretKey = maybeString(m["secret-key"], "")
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

	regionName := maybeString(m["region"], "us-east-1")
	if _, ok := aws.Regions[regionName]; !ok {
		return nil, fmt.Errorf("invalid region name %q", regionName)
	}
	c.region = regionName
	return c, nil
}

func maybeString(x interface{}, dflt string) string {
	if x == nil {
		return dflt
	}
	return x.(string)
}
