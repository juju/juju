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
		"type":      "dummy",
		"zookeeper": false,
	}
	env, err := environs.NewEnviron("dummy", config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(false), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewEnviron("wondercloud", nil)
	c.Assert(err, ErrorMatches, "no registered provider for kind:.*")
	c.Assert(env, IsNil)
}
