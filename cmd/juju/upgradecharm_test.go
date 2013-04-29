package main

import (
	. "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/testing"
)

type UpgradeCharmErrorsSuite struct {
	jujutesting.RepoSuite
}

var _ = Suite(&UpgradeCharmErrorsSuite{})

func runUpgradeCharm(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &UpgradeCharmCommand{}, args)
	return err
}

func (s *UpgradeCharmErrorsSuite) TestInvalidArgs(c *C) {
	err := runUpgradeCharm(c)
	c.Assert(err, ErrorMatches, "no service specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, ErrorMatches, `invalid service name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, ErrorMatches, `unrecognized args: \["bar"\]`)
}
