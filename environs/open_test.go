package environs_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
)

type OpenSuite struct{}

var _ = Suite(&OpenSuite{})

func (OpenSuite) TestNewDummyEnviron(c *C) {
	// matches *Settings.Map()
	config := map[string]interface{}{
		"name":             "foo",
		"type":             "dummy",
		"state-server":     false,
		"authorized-keys":  "i-am-a-key",
		"admin-secret":     "foo",
		"root-cert":        testing.RootCertPEM,
		"root-private-key": "",
	}
	env, err := environs.NewFromAttrs(config)
	c.Assert(err, IsNil)
	c.Assert(env.Bootstrap(false, nil), IsNil)
}

func (OpenSuite) TestNewUnknownEnviron(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":             "foo",
		"type":             "wondercloud",
		"authorized-keys":  "i-am-a-key",
		"root-cert":        testing.RootCertPEM,
		"root-private-key": "",
	})
	c.Assert(err, ErrorMatches, "no registered provider for.*")
	c.Assert(env, IsNil)
}
