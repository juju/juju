package main_test

import (
	. "launchpad.net/gocheck"
	main "launchpad.net/juju/go/cmd/juju"
)

type BootstrapSuite struct{}

var _ = Suite(&BootstrapSuite{})

func (s *BootstrapSuite) TestEnvironment(c *C) {
	bc := main.NewBootstrapCommand()
	c.Assert(bc.Environment(), Equals, "")

	err := bc.Parse([]string{})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment(), Equals, "")

	err = bc.Parse([]string{"hotdog"})
	c.Assert(err, ErrorMatches, `Unknown args: \[hotdog\]`)
	c.Assert(bc.Environment(), Equals, "")

	err = bc.Parse([]string{"-e", "walthamstow"})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment(), Equals, "walthamstow")

	err = bc.Parse([]string{"--environment", "peckham"})
	c.Assert(err, IsNil)
	c.Assert(bc.Environment(), Equals, "peckham")
}
