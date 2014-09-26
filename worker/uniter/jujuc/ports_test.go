// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type PortsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&PortsSuite{})

var portsTests = []struct {
	cmd    []string
	expect set.Strings
}{
	{[]string{"open-port", "80"}, set.NewStrings("80/tcp")},
	{[]string{"open-port", "99/tcp"}, set.NewStrings("80/tcp", "99/tcp")},
	{[]string{"open-port", "100-200"}, set.NewStrings("80/tcp", "99/tcp", "100-200/tcp")},
	{[]string{"open-port", "443/udp"}, set.NewStrings("80/tcp", "99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "80/TCP"}, set.NewStrings("99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "100-200/tcP"}, set.NewStrings("99/tcp", "443/udp")},
	{[]string{"close-port", "443"}, set.NewStrings("99/tcp", "443/udp")},
	{[]string{"close-port", "443/udp"}, set.NewStrings("99/tcp")},
	{[]string{"open-port", "123/udp"}, set.NewStrings("99/tcp", "123/udp")},
	{[]string{"close-port", "9999/UDP"}, set.NewStrings("99/tcp", "123/udp")},
}

func (s *PortsSuite) TestOpenClose(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsTests {
		com, err := jujuc.NewCommand(hctx, cmdString(t.cmd[0]))
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(hctx.ports, gc.DeepEquals, t.expect)
	}
}

var badPortsTests = []struct {
	args []string
	err  string
}{
	{nil, "no port or range specified"},
	{[]string{"0"}, `port must be in the range \[1, 65535\]; got "0"`},
	{[]string{"65536"}, `port must be in the range \[1, 65535\]; got "65536"`},
	{[]string{"two"}, `expected <port>\[/<protocol>\] or <from>-<to>\[/<protocol>\]; got "two"`},
	{[]string{"80/http"}, `protocol must be "tcp" or "udp"; got "http"`},
	{[]string{"blah/blah/blah"}, `expected <port>\[/<protocol>\] or <from>-<to>\[/<protocol>\]; got "blah/blah/blah"`},
	{[]string{"123", "haha"}, `unrecognized args: \["haha"\]`},
	{[]string{"1-0"}, `invalid port range 1-0/tcp; expected fromPort <= toPort`},
	{[]string{"-42"}, `flag provided but not defined: -4`},
	{[]string{"99999/UDP"}, `port must be in the range \[1, 65535\]; got "99999"`},
	{[]string{"9999/foo"}, `protocol must be "tcp" or "udp"; got "foo"`},
	{[]string{"80-90/http"}, `protocol must be "tcp" or "udp"; got "http"`},
	{[]string{"20-10/tcp"}, `invalid port range 20-10/tcp; expected fromPort <= toPort`},
}

func (s *PortsSuite) TestBadArgs(c *gc.C) {
	for _, name := range []string{"open-port", "close-port"} {
		for _, t := range badPortsTests {
			hctx := s.GetHookContext(c, -1, "")
			com, err := jujuc.NewCommand(hctx, cmdString(name))
			c.Assert(err, gc.IsNil)
			err = testing.InitCommand(com, t.args)
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *PortsSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	open, err := jujuc.NewCommand(hctx, cmdString("open-port"))
	c.Assert(err, gc.IsNil)
	flags := testing.NewFlagSet()
	c.Assert(string(open.Info().Help(flags)), gc.Equals, `
usage: open-port <port>[/<protocol>] or <from>-<to>[/<protocol>]
purpose: register a port or range to open

The port range will only be open while the service is exposed.
`[1:])

	close, err := jujuc.NewCommand(hctx, cmdString("close-port"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(close.Info().Help(flags)), gc.Equals, `
usage: close-port <port>[/<protocol>] or <from>-<to>[/<protocol>]
purpose: ensure a port or range is always closed
`[1:])
}

// Since the deprecation warning gets output during Run, we really need
// some valid commands to run
var portsFormatDeprectaionTests = []struct {
	cmd []string
}{
	{[]string{"open-port", "--format", "foo", "80"}},
	{[]string{"close-port", "--format", "foo", "80/TCP"}},
}

func (s *PortsSuite) TestOpenCloseDeprecation(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsFormatDeprectaionTests {
		name := t.cmd[0]
		com, err := jujuc.NewCommand(hctx, cmdString(name))
		c.Assert(err, gc.IsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(testing.Stdout(ctx), gc.Equals, "")
		c.Assert(testing.Stderr(ctx), gc.Equals, "--format flag deprecated for command \""+name+"\"")
	}
}
