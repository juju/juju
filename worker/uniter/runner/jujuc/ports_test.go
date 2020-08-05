// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type PortsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&PortsSuite{})

var portsTests = []struct {
	cmd    []string
	expect []network.PortRange
}{
	{[]string{"open-port", "80"}, makeRanges("80/tcp")},
	{[]string{"open-port", "99/tcp"}, makeRanges("80/tcp", "99/tcp")},
	{[]string{"open-port", "100-200"}, makeRanges("80/tcp", "99/tcp", "100-200/tcp")},
	{[]string{"open-port", "443/udp"}, makeRanges("80/tcp", "99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "80/TCP"}, makeRanges("99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "100-200/tcP"}, makeRanges("99/tcp", "443/udp")},
	{[]string{"close-port", "443"}, makeRanges("99/tcp", "443/udp")},
	{[]string{"close-port", "443/udp"}, makeRanges("99/tcp")},
	{[]string{"open-port", "123/udp"}, makeRanges("99/tcp", "123/udp")},
	{[]string{"close-port", "9999/UDP"}, makeRanges("99/tcp", "123/udp")},
	{[]string{"open-port", "icmp"}, makeRanges("icmp", "99/tcp", "123/udp")},
}

func makeRanges(stringRanges ...string) []network.PortRange {
	var results []network.PortRange
	for _, s := range stringRanges {
		results = append(results, network.MustParsePortRange(s))
	}
	network.SortPortRanges(results)
	return results
}

func (s *PortsSuite) TestOpenClose(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsTests {
		com, err := jujuc.NewCommand(hctx, cmdString(t.cmd[0]))
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Check(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		hctx.info.CheckPortRanges(c, map[string][]network.PortRange{
			"": t.expect,
		})
	}
}

func (s *PortsSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	open, err := jujuc.NewCommand(hctx, cmdString("open-port"))
	c.Assert(err, jc.ErrorIsNil)
	flags := cmdtesting.NewFlagSet()
	c.Assert(string(open.Info().Help(flags)), gc.Equals, `
Usage: open-port <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp

Summary:
register a port or range to open

Details:
The port range will only be open while the application is exposed.
`[1:])

	close, err := jujuc.NewCommand(hctx, cmdString("close-port"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(close.Info().Help(flags)), gc.Equals, `
Usage: close-port <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp

Summary:
ensure a port or range is always closed
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
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "--format flag deprecated for command \""+name+"\"")
	}
}
