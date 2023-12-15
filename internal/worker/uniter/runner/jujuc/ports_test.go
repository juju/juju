// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type PortsSuite struct {
	ContextSuite
}

var _ = gc.Suite(&PortsSuite{})

var portsTests = []struct {
	cmd    []string
	expect network.GroupedPortRanges
}{
	{[]string{"open-port", "80"}, makeAllEndpointsRanges("80/tcp")},
	{[]string{"open-port", "99/tcp"}, makeAllEndpointsRanges("80/tcp", "99/tcp")},
	{[]string{"open-port", "100-200"}, makeAllEndpointsRanges("80/tcp", "99/tcp", "100-200/tcp")},
	{[]string{"open-port", "443/udp"}, makeAllEndpointsRanges("80/tcp", "99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "80/TCP"}, makeAllEndpointsRanges("99/tcp", "100-200/tcp", "443/udp")},
	{[]string{"close-port", "100-200/tcP"}, makeAllEndpointsRanges("99/tcp", "443/udp")},
	{[]string{"close-port", "443"}, makeAllEndpointsRanges("99/tcp", "443/udp")},
	{[]string{"close-port", "443/udp"}, makeAllEndpointsRanges("99/tcp")},
	{[]string{"open-port", "123/udp"}, makeAllEndpointsRanges("99/tcp", "123/udp")},
	{[]string{"close-port", "9999/UDP"}, makeAllEndpointsRanges("99/tcp", "123/udp")},
	{[]string{"open-port", "icmp"}, makeAllEndpointsRanges("icmp", "99/tcp", "123/udp")},
	// Tests with --endpoints.
	{[]string{"open-port", "--endpoints", "foo,bar", "1337/tcp"}, network.GroupedPortRanges{
		// Pre-existing ports from previous tests
		"": []network.PortRange{
			network.MustParsePortRange("icmp"),
			network.MustParsePortRange("99/tcp"),
			network.MustParsePortRange("123/udp"),
		},
		// Endpoint-specific ports
		"foo": []network.PortRange{network.MustParsePortRange("1337/tcp")},
		"bar": []network.PortRange{network.MustParsePortRange("1337/tcp")},
	}},
	{[]string{"close-port", "--endpoints", "foo", "1337/tcp"}, network.GroupedPortRanges{
		"": []network.PortRange{
			network.MustParsePortRange("icmp"),
			network.MustParsePortRange("99/tcp"),
			network.MustParsePortRange("123/udp"),
		},
		"foo": []network.PortRange{
			// Removed
		},
		"bar": []network.PortRange{network.MustParsePortRange("1337/tcp")},
	}},
}

func makeAllEndpointsRanges(stringRanges ...string) network.GroupedPortRanges {
	var results []network.PortRange
	for _, s := range stringRanges {
		results = append(results, network.MustParsePortRange(s))
	}
	network.SortPortRanges(results)
	return network.GroupedPortRanges{
		"": results,
	}
}

func (s *PortsSuite) TestOpenClose(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsTests {
		com, err := jujuc.NewCommand(hctx, t.cmd[0])
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Check(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		hctx.info.CheckPortRanges(c, t.expect)
	}
}

func (s *PortsSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	open, err := jujuc.NewCommand(hctx, "open-port")
	c.Assert(err, jc.ErrorIsNil)
	flags := cmdtesting.NewFlagSet()
	c.Assert(string(open.Info().Help(flags)), gc.Equals, `
Usage: open-port <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp

Summary:
register a request to open a port or port range

Details:
open-port registers a request to open the specified port or port range.

By default, the specified port or port range will be opened for all defined
application endpoints. The --endpoints option can be used to constrain the
open request to a comma-delimited list of application endpoints.
`[1:])

	close, err := jujuc.NewCommand(hctx, "close-port")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(close.Info().Help(flags)), gc.Equals, `
Usage: close-port <port>[/<protocol>] or <from>-<to>[/<protocol>] or icmp

Summary:
register a request to close a port or port range

Details:
close-port registers a request to close the specified port or port range.

By default, the specified port or port range will be closed for all defined
application endpoints. The --endpoints option can be used to constrain the
close request to a comma-delimited list of application endpoints.
`[1:])
}

// Since the deprecation warning gets output during Run, we really need
// some valid commands to run
var portsFormatDeprecationTests = []struct {
	cmd []string
}{
	{[]string{"open-port", "--format", "foo", "80"}},
	{[]string{"close-port", "--format", "foo", "80/TCP"}},
}

func (s *PortsSuite) TestOpenCloseDeprecation(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsFormatDeprecationTests {
		name := t.cmd[0]
		com, err := jujuc.NewCommand(hctx, name)
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "--format flag deprecated for command \""+name+"\"")
	}
}
