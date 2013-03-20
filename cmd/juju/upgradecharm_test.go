package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type UpgradeCharmSuite struct {
	repoSuite
}

var _ = Suite(&UpgradeCharmSuite{})

func runUpgradeCharm(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &UpgradeCharmCommand{}, args)
	return err
}

func (s *UpgradeCharmSuite) TestInvalidArgs(c *C) {
	err := runUpgradeCharm(c)
	c.Assert(err, ErrorMatches, "no service specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, ErrorMatches, `invalid service name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmSuite) TestInvalidService(c *C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(err, ErrorMatches, `service "phony" not found`)
}

func (s *UpgradeCharmSuite) TestSuccess(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	riak, err := s.State.Service("riak")
	c.Assert(err, IsNil)
	c.Assert(riak.Life(), Equals, state.Alive)
	ch, forced, err := riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 7)
	c.Assert(forced, Equals, false)
	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	err = riak.Refresh()
	c.Assert(err, IsNil)
	ch, forced, err = riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 8)
	c.Assert(forced, Equals, false)
}
