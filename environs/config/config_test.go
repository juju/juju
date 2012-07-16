package config_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type ConfigSuite struct{}

var _ = Suite(&ConfigSuite{})

type attrs map[string]interface{}

var configTests = []struct{ attrs map[string]interface{}; err string }{
{
	// A good configuration.
	attrs{
		"type": "my-type",
		"name": "my-name",
		"default-series": "my-series",
	},
	"",
}, {
	// Inherit the current series if none is provided
	attrs{
		"type": "my-type",
		"name": "my-name",
	},
	"",
}, {
	// Missing type.
	attrs{
		"name": "my-name",
	},
	"type: expected string, got nothing",
}, {
	// Missing name.
	attrs{
		"type": "my-type",
	},
	"name: expected string, got nothing",
}}

func (*ConfigSuite) TestConfig(c *C) {
	for _, test := range configTests {
		cfg, err := config.New(test.attrs)
		if test.err != "" {
			c.Assert(err, ErrorMatches, test.err)
			continue
		} else if err != nil {
			c.Fatalf("error with config %#v: %v", test.attrs, err)
		}

		etype, _ := test.attrs["type"].(string)
		ename, _ := test.attrs["name"].(string)
		c.Assert(cfg.Type(), Equals, etype)
		c.Assert(cfg.Name(), Equals, ename)

		if eseries, ok := test.attrs["default-series"].(string); ok {
			c.Assert(cfg.DefaultSeries(), Equals, eseries)
		} else {
			c.Assert(cfg.DefaultSeries(), Equals, config.CurrentSeries)
		}
	}
}

