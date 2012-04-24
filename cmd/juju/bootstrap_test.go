package main_test

import (
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
)

type BootstrapSuite struct{}

var _ = Suite(&BootstrapSuite{})

func initCmd(c cmd.Command, args []string) error {
	return c.Init(gnuflag.NewFlagSet("", gnuflag.ContinueOnError), args)
}

func checkEnv(c *C, args []string, expect string) {
	bc := &main.BootstrapCommand{}
	err := initCmd(bc, args)
	c.Assert(err, IsNil)
	c.Assert(bc.Environment, Equals, expect)
}

func (s *BootstrapSuite) TestEnvironment(c *C) {
	bc := &main.BootstrapCommand{}
	c.Assert(bc.Environment, Equals, "")
	err := initCmd(bc, []string{"hotdog"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[hotdog\]`)
	c.Assert(bc.Environment, Equals, "")

	checkEnv(c, []string{}, "")
	checkEnv(c, []string{"-e", "walthamstow"}, "walthamstow")
	checkEnv(c, []string{"--environment", "peckham"}, "peckham")
}
