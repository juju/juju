package server_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
)

type PortsSuite struct {
	UnitFixture
}

var _ = Suite(&PortsSuite{})

var portsTests = []struct {
	cmd  []string
	open []state.Port
}{
	{[]string{"open-port", "80"}, []state.Port{{"tcp", 80}}},
	{[]string{"open-port", "99/tcp"}, []state.Port{{"tcp", 80}, {"tcp", 99}}},
	{[]string{"close-port", "80/TCP"}, []state.Port{{"tcp", 99}}},
	{[]string{"open-port", "123/udp"}, []state.Port{{"tcp", 99}, {"udp", 123}}},
	{[]string{"close-port", "9999/UDP"}, []state.Port{{"tcp", 99}, {"udp", 123}}},
}

func (s *PortsSuite) TestOpenClose(c *C) {
	for _, t := range portsTests {
		com, err := s.ctx.NewCommand(t.cmd[0])
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		c.Assert(bufferString(ctx.Stderr), Equals, "")
		open, err := s.unit.OpenPorts()
		c.Assert(err, IsNil)
		c.Assert(open, DeepEquals, t.open)
	}
}

var badPortsTests = []struct {
	args []string
	err  string
}{
	{nil, "no port specified"},
	{[]string{"0"}, `port must be in the range \[1, 65535\]; got "0"`},
	{[]string{"65536"}, `port must be in the range \[1, 65535\]; got "65536"`},
	{[]string{"two"}, `port must be in the range \[1, 65535\]; got "two"`},
	{[]string{"80/http"}, `protocol must be "tcp" or "udp"; got "http"`},
	{[]string{"blah/blah/blah"}, `expected <port>\[/<protocol>\]; got "blah/blah/blah"`},
	{[]string{"123", "haha"}, `unrecognized args: \["haha"\]`},
}

func (s *PortsSuite) TestBadArgs(c *C) {
	for _, name := range []string{"open-port", "close-port"} {
		for _, t := range badPortsTests {
			com, err := s.ctx.NewCommand(name)
			c.Assert(err, IsNil)
			err = com.Init(dummyFlagSet(), t.args)
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}

func (s *PortsSuite) TestHelp(c *C) {
	open, err := s.ctx.NewCommand("open-port")
	c.Assert(err, IsNil)
	c.Assert(string(open.Info().Help(dummyFlagSet())), Equals, `
usage: open-port <port>[/<protocol>]
purpose: register a port to open

The port will only be open while the service is exposed.
`[1:])

	close, err := s.ctx.NewCommand("close-port")
	c.Assert(err, IsNil)
	c.Assert(string(close.Info().Help(dummyFlagSet())), Equals, `
usage: close-port <port>[/<protocol>]
purpose: ensure a port is always closed
`[1:])
}

func (s *PortsSuite) TestUnitCommands(c *C) {
	s.AssertUnitCommand(c, "open-port")
	s.AssertUnitCommand(c, "close-port")
}
