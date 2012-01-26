package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/juju/go/schema"
)

// providerConfig is a placeholder for any config information
// that we will have in a configuration file.
type providerConfig struct {
	region string
	auth   aws.Auth
	bucket string
}

type checker struct{}

func (checker) Coerce(v interface{}, path []string) (interface{}, error) {
	return &providerConfig{}, nil
}

// TODO move these known strings into goamz/aws
var Regions = map[string]aws.Region{
	"ap-northeast-1": aws.APNortheast,
	"ap-southeast-1": aws.APSoutheast,
	"eu-west-1":      aws.EUWest,
	"us-east-1":      aws.USEast,
	"us-west-1":      aws.USWest,
}

func (environProvider) ConfigChecker() schema.Checker {
	return combineCheckers(
		schema.FieldMap(
			schema.Fields{
				"access-key":     schema.String(),
				"secret-key":     schema.String(),
				"region":         schema.String(),
				"control-bucket": schema.String(),
			}, []string{
				"access-key",
				"secret-key",
				"region",
			},
		),
		checkerFunc(func(v interface{}, path []string) (newv interface{}, err error) {
			m := v.(schema.MapType)
			var c providerConfig

			c.bucket = m["control-bucket"].(string)
			c.auth.AccessKey = maybeString(m["access-key"], "")
			c.auth.SecretKey = maybeString(m["secret-key"], "")
			if c.auth.AccessKey == "" || c.auth.SecretKey == "" {
				if c.auth.AccessKey != "" {
					return nil, fmt.Errorf("environment has access-key but no secret-key")
				}
				if c.auth.SecretKey != "" {
					return nil, fmt.Errorf("environment has secret-key but no access-key")
				}
				var err error
				c.auth, err = aws.EnvAuth()
				if err != nil {
					return nil, err
				}
			}

			regionName := maybeString(m["region"], "us-east-1")
			if _, ok := Regions[regionName]; !ok {
				return nil, fmt.Errorf("invalid region name %q", regionName)
			}
			c.region = regionName
			return &c, nil
		}),
	)
}

func maybeString(x interface{}, dflt string) string {
	if x == nil {
		return dflt
	}
	return x.(string)
}
