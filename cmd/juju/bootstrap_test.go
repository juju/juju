package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
)

type BootstrapSuite struct{}

var _ = Suite(&BootstrapSuite{})

func (s *BootstrapSuite) TestEnvironment(c *C) {
	bc := &main.BootstrapCommand{}
	c.Assert(bc.Environment, Equals, "")

	err := cmd.Parse(bc, true, []string{})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment, Equals, "")

	err = cmd.Parse(bc, true, []string{"hotdog"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[hotdog\]`)
	c.Assert(bc.Environment, Equals, "")

	err = cmd.Parse(bc, true, []string{"-e", "walthamstow"})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment, Equals, "walthamstow")

	err = cmd.Parse(bc, true, []string{"--environment", "peckham"})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment, Equals, "peckham")
}
