package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	_ "launchpad.net/juju/go/environs/dummy"
)

type OpenSuite struct{}

var _ = Suite(&OpenSuite{})

func (OpenSuite) TestNewDummyEnviron(c *C) {
	// matches *ConfigNode.Map()
	config := map[string]interface{}{
		"name":      "foo",
		"type":      "dummy",
		"zookeeper": false,
	}
	env, err := environs.NewEnviron(config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(false), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewEnviron(map[string]interface{}{"type": "wondercloud"})
	c.Assert(err, ErrorMatches, "error validating environment: no registered provider for.*")
	c.Assert(env, IsNil)
}

func (OpenSuite) TestValidNewConfig(c *C) {
	cfg, err := environs.NewConfig(map[string]interface{}{
		"name":      "test",
		"type":      "dummy",
		"zookeeper": false,
	})
	c.Assert(err, IsNil)
	c.Assert(cfg, NotNil)

	env, err := cfg.Open()
	c.Assert(err, IsNil)
	c.Assert(env, NotNil)
}

func (OpenSuite) TestInvalidNewConfig(c *C) {
	cfg, err := environs.NewConfig(map[string]interface{}{
		"name": "test",
		"type": "dummy",
		// zookeeper is missing
	})
	c.Assert(err, ErrorMatches, "zookeeper: expected false, got nothing")
	c.Assert(cfg, IsNil)
}
