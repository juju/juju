package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/schema"
)

func init() {
	juju.RegisterProvider("ec2", provider{})
}

var regions = map[string]aws.Region{
	"ap-northeast-1": aws.APNortheast,
	"ap-southeast-1": aws.APSoutheast,
	"eu-west-1":      aws.EUWest,
	"us-east-1":      aws.USEast,
	"us-west-1":      aws.USWest,
}

type providerConfig struct {
	controlBucket       string
	adminSecret         string
	accessKey           string
	secretKey           string
	region              aws.Region
	defaultInstanceType string
	defaultAMI          string
	ec2URI              string
	s3URI               string
	placement           string
	defaultSeries       string
}

func maybeString(x interface{}) string {
	if x == nil {
		return ""
	}
	return x.(string)
}

type provider struct{}

func (provider) ConfigChecker() schema.Checker {
	return combineCheckers(
		schema.FieldMap(
			schema.Fields{
				"access-key":            schema.String(),
				"admin-secret":          schema.String(),
				"control-bucket":        schema.String(),
				"default-ami":           schema.String(),
				"default-instance-type": schema.String(),
				"default-series":        schema.String(),
				"ec2-uri":               schema.String(),
				"placement":             oneOf("unassigned", "local"),
				"region":                schema.String(),
				"s3-uri":                schema.String(),
				"secret-key":            schema.String(),
			}, []string{
				"access-key",
				"default-ami",
				"default-instance-type",
				"default-series",
				"ec2-uri",
				"placement",
				"region",
				"s3-uri",
				"secret-key",
			},
		),
		checkerFunc(func(v interface{}, path []string) (newv interface{}, err error) {
			m := v.(schema.MapType)
			var c providerConfig
			c.controlBucket = maybeString(m["control-bucket"])
			c.adminSecret = maybeString(m["admin-secret"])
			c.accessKey = maybeString(m["access-key"])
			c.secretKey = maybeString(m["secret-key"])
			regionName := maybeString(m["region"])
			if regionName == "" {
				regionName = "us-east-1"
			}
			if r, ok := regions[regionName]; ok {
				c.region = r
			} else {
				return nil, fmt.Errorf("invalid region name %q", regionName)
			}
			c.defaultInstanceType = maybeString(m["default-instance-type"])
			if c.defaultInstanceType == "" {
				c.defaultInstanceType = "m1.small"
			}
			c.defaultAMI = maybeString(m["default-ami"])
			c.ec2URI = maybeString(m["ec2-uri"])
			c.s3URI = maybeString(m["s3-uri"])
			c.placement = maybeString(m["placement"])
			c.defaultSeries = maybeString(m["default-series"])
			// TODO more checking here?
			return &c, nil
		}),
	)
}
