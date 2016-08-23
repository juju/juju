// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type NetworkGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&NetworkGetSuite{})

func (s *NetworkGetSuite) createCommand(c *gc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")

	presetBindings := make(map[string][]params.NetworkConfig)
	presetBindings["known-relation"] = []params.NetworkConfig{
		{Address: "10.10.0.23"},
		{Address: "192.168.1.111"},
	}
	presetBindings["known-extra"] = []params.NetworkConfig{
		{Address: "10.20.1.42"},
		{Address: "fc00::1/64"},
	}
	presetBindings["valid-no-config"] = nil
	// Simulate known but unspecified bindings.
	presetBindings["known-unbound"] = []params.NetworkConfig{
		{Address: "10.33.1.8"}, // Simulate preferred private address will be used for these.
	}
	hctx.info.NetworkInterface.BindingsToNetworkConfigs = presetBindings

	com, err := jujuc.NewCommand(hctx, cmdString("network-get"))
	c.Assert(err, jc.ErrorIsNil)
	return com
}

func (s *NetworkGetSuite) TestNetworkGet(c *gc.C) {
	for i, t := range []struct {
		summary  string
		args     []string
		code     int
		out      string
		checkctx func(*gc.C, *cmd.Context)
	}{{
		summary: "no arguments",
		code:    2,
		out:     `no arguments specified`,
	}, {
		summary: "empty binding name specified",
		code:    2,
		args:    []string{""},
		out:     `no binding name specified`,
	}, {
		summary: "binding name given, no --primary-address given",
		code:    2,
		args:    []string{"foo"},
		out:     `--primary-address is currently required`,
	}, {
		summary: "unknown binding given, with --primary-address",
		args:    []string{"unknown", "--primary-address"},
		code:    1,
		out:     "insert server error for unknown binding here",
	}, {
		summary: "valid arguments, API server returns no config",
		args:    []string{"valid-no-config", "--primary-address"},
		code:    1,
		out:     `no network config found for binding "valid-no-config"`,
	}, {
		summary: "explicitly bound, extra-binding name given with --primary-address",
		args:    []string{"known-extra", "--primary-address"},
		out:     "10.20.1.42",
	}, {
		summary: "explicitly bound relation name given with --primary-address",
		args:    []string{"known-relation", "--primary-address"},
		out:     "10.10.0.23",
	}, {
		summary: "implicitly bound binding name given with --primary-address",
		args:    []string{"known-unbound", "--primary-address"},
		out:     "10.33.1.8", // preferred private address used for unspecified bindings.
	}} {
		c.Logf("test %d: %s", i, t.summary)
		com := s.createCommand(c)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if expect != "" {
				expect = expect + "\n"
			}
			c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Check(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.out)
			c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

func (s *NetworkGetSuite) TestHelp(c *gc.C) {

	var helpTemplate = `
Usage: network-get [options] <binding-name> --primary-address

Summary:
get network config

Options:
--format  (= smart)
    Specify output format (json|smart|yaml)
-o, --output (= "")
    Specify an output file
--primary-address  (= false)
    get the primary address for the binding

Details:
network-get returns the network config for a given binding name. The only
supported flag for now is --primary-address, which is required and returns
the IP address the local unit should advertise as its endpoint to its peers.
`[1:]

	com := s.createCommand(c)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(bufferString(ctx.Stdout), gc.Equals, helpTemplate)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
