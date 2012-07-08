package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/schema"
)

// providerConfig is a placeholder for any config information
// that we will have in a configuration file.
type providerConfig struct {
	name           string
	region         string
	auth           aws.Auth
	bucket         string
	publicBucket   string
	authorizedKeys string
}

var configChecker = schema.StrictFieldMap(
	schema.Fields{
		"name":                 schema.String(),
		"type":                 schema.Const("ec2"),
		"access-key":           schema.String(),
		"secret-key":           schema.String(),
		"region":               schema.String(),
		"control-bucket":       schema.String(),
		"public-bucket":        schema.String(),
		"authorized-keys":      schema.String(),
		"authorized-keys-path": schema.String(),
	}, []string{
		"access-key",
		"secret-key",
		"region",
		"authorized-keys",
		"authorized-keys-path",
		"public-bucket",
	},
)

func (p environProvider) NewConfig(config map[string]interface{}) (cfg environs.EnvironConfig, err error) {
	v, err := configChecker.Coerce(config, nil)
	if err != nil {
		return nil, err
	}
	m := v.(schema.MapType)
	var c providerConfig

	c.name = m["name"].(string)
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
			return
		}
	}

	regionName := maybeString(m["region"], "us-east-1")
	if _, ok := aws.Regions[regionName]; !ok {
		return nil, fmt.Errorf("invalid region name %q", regionName)
	}
	c.region = regionName
	c.authorizedKeys = maybeString(m["authorized-keys"], "")
	authorizedKeysPath := maybeString(m["authorized-keys-path"], "")
	if c.authorizedKeys == "" {
		c.authorizedKeys, err = authorizedKeys(authorizedKeysPath)
		if err != nil {
			return nil, err
		}
	} else if authorizedKeysPath != "" {
		return nil, fmt.Errorf("environment has both authorized-keys and authorized-keys-path")
	}
	return &c, nil
}

func maybeString(x interface{}, dflt string) string {
	if x == nil {
		return dflt
	}
	return x.(string)
}
