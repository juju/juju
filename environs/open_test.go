package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/dummy"
)

type OpenSuite struct{}

var _ = Suite(&OpenSuite{})

func (OpenSuite) TestNewDummyEnviron(c *C) {
	// matches *ConfigNode.Map()
	config := map[string]interface{}{
		"name":            "foo",
		"type":            "dummy",
		"zookeeper":       false,
		"authorized-keys": "i-am-a-key",
	}
	env, err := environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(false), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name": "foo",
		"type": "wondercloud",
		"authorized-keys": "i-am-a-key",
	})
	c.Assert(err, ErrorMatches, "no registered provider for.*")
	c.Assert(env, IsNil)
}
