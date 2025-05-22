// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type PortsSuite struct {
	ContextSuite
}

func TestPortsSuite(t *stdtesting.T) {
	tc.Run(t, &PortsSuite{})
}

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

func (s *PortsSuite) TestOpenClose(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsTests {
		com, err := jujuc.NewCommand(hctx, t.cmd[0])
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Check(code, tc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), tc.Equals, "")
		c.Assert(bufferString(ctx.Stderr), tc.Equals, "")
		hctx.info.CheckPortRanges(c, t.expect)
	}
}

// Since the deprecation warning gets output during Run, we really need
// some valid commands to run
var portsFormatDeprecationTests = []struct {
	cmd []string
}{
	{[]string{"open-port", "--format", "foo", "80"}},
	{[]string{"close-port", "--format", "foo", "80/TCP"}},
}

func (s *PortsSuite) TestOpenCloseDeprecation(c *tc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsFormatDeprecationTests {
		name := t.cmd[0]
		com, err := jujuc.NewCommand(hctx, name)
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.cmd[1:])
		c.Assert(code, tc.Equals, 0)
		c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
		c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "--format flag deprecated for command \""+name+"\"")
	}
}
