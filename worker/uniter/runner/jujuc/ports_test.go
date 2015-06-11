// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"strconv"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
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
}

func makeRanges(stringRanges ...string) []network.PortRange {
	var results []network.PortRange
	for _, s := range stringRanges {
		if strings.Contains(s, "-") {
			parts := strings.Split(s, "-")
			fromPort, _ := strconv.Atoi(parts[0])
			parts = strings.Split(parts[1], "/")
			toPort, _ := strconv.Atoi(parts[0])
			proto := parts[1]
			results = append(results, network.PortRange{
				FromPort: fromPort,
				ToPort:   toPort,
				Protocol: proto,
			})
		} else {
			parts := strings.Split(s, "/")
			port, _ := strconv.Atoi(parts[0])
			proto := parts[1]
			results = append(results, network.PortRange{
				FromPort: port,
				ToPort:   port,
				Protocol: proto,
			})
		}
	}
	network.SortPortRanges(results)
	return results
}

func (s *PortsSuite) TestOpenClose(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	for _, t := range portsTests {
		com, err := jujuc.NewCommand(hctx, cmdString(t.cmd[0]))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		hctx.info.CheckPorts(c, t.expect)
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
			c.Assert(err, jc.ErrorIsNil)
			err = testing.InitCommand(com, t.args)
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

func (s *PortsSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	open, err := jujuc.NewCommand(hctx, cmdString("open-port"))
	c.Assert(err, jc.ErrorIsNil)
	flags := testing.NewFlagSet()
	c.Assert(string(open.Info().Help(flags)), gc.Equals, `
usage: open-port <port>[/<protocol>] or <from>-<to>[/<protocol>]
purpose: register a port or range to open

The port range will only be open while the service is exposed.
`[1:])

	close, err := jujuc.NewCommand(hctx, cmdString("close-port"))
	c.Assert(err, jc.ErrorIsNil)
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
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.cmd[1:])
		c.Assert(code, gc.Equals, 0)
		c.Assert(testing.Stdout(ctx), gc.Equals, "")
		c.Assert(testing.Stderr(ctx), gc.Equals, "--format flag deprecated for command \""+name+"\"")
	}
}
