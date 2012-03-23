package main_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	main "launchpad.net/juju/go/cmd/juju"
)

type suite struct{}

var _ = Suite(suite{})

func (suite) Test

func checkEnv(c *C, args []string, expect string) {
	bc := &main.BootstrapCommand{}
	err := cmd.Parse(bc, args)
	c.Assert(err, IsNil)
	c.Assert(bc.Environment, Equals, expect)
}

type cmdParseTest = []struct {
	c cmd.Command
	args []string
	check func(*C, cmd.Command, error)
} {{
	&BootstrapCommand{},
	[]string{"hotdog"},
	func(c *C, com cmd.Command, err error){
		c.Assert(err, ErrorMatches, `unrecognised args: \[hotdog\]`)
	},
}, {
	&BootstrapCommand{},
	[]string{},
}, {
	&BootstrapCommand{},
	[]string{"-e", "walthamstow"},
}, {
	&BootstrapCommand{},
	[]
	
		bc := com.(*BootstrapCommand)
		c.Assert(b

func (suite) TestBootstrapEnvironment(c *C) {
	bc := &main.BootstrapCommand{}
	c.Assert(bc.Environment, Equals, "")
	err := cmd.Parse(bc, []string{"hotdog"})
	c.Assert(err, ErrorMatches, `unrecognised args: \[hotdog\]`)
	c.Assert(bc.Environment, Equals, "")

	checkEnv(c, []string{}, "")
	checkEnv(c, []string{"-e", "walthamstow"}, "walthamstow")
	checkEnv(c, []string{"--environment", "peckham"}, "peckham")
}
