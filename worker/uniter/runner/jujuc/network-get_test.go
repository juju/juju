// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type NetworkGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&NetworkGetSuite{})

func (s *NetworkGetSuite) createCommand(c *gc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")

	presetBindings := make(map[string]params.NetworkInfoResult)
	presetBindings["known-relation"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{MACAddress: "00:11:22:33:44:00",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{
						Address: "10.10.0.23",
						CIDR:    "10.10.0.0/24",
					},
					{
						Address: "192.168.1.111",
						CIDR:    "192.168.1.0/24",
					},
				},
			},
			{MACAddress: "00:11:22:33:44:11",
				InterfaceName: "eth1",
				Addresses: []params.InterfaceAddress{
					{
						Address: "10.10.1.23",
						CIDR:    "10.10.1.0/24",
					},
					{
						Address: "192.168.2.111",
						CIDR:    "192.168.2.0/24",
					},
				},
			},
		},
	}
	presetBindings["known-extra"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{MACAddress: "00:11:22:33:44:22",
				InterfaceName: "eth2",
				Addresses: []params.InterfaceAddress{
					{
						Address: "10.20.1.42",
						CIDR:    "10.20.1.42/24",
					},
					{
						Address: "fc00::1",
						CIDR:    "fc00::/64",
					},
				},
			},
		},
	}
	presetBindings["valid-no-config"] = params.NetworkInfoResult{}
	// Simulate known but unspecified bindings.
	presetBindings["known-unbound"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{MACAddress: "00:11:22:33:44:33",
				InterfaceName: "eth3",
				Addresses: []params.InterfaceAddress{
					{
						Address: "10.33.1.8",
						CIDR:    "10.33.1.8/24",
					},
				},
			},
		},
	}
	hctx.info.NetworkInterface.NetworkInfoResults = presetBindings

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
		summary: "unknown binding given, no --primary-address given",
		code:    1,
		args:    []string{"unknown"},
		out:     `no network config found for binding "unknown"`,
	}, {
		summary: "unknown binding given, with --primary-address",
		args:    []string{"unknown", "--primary-address"},
		code:    1,
		out:     `no network config found for binding "unknown"`,
	}, {
		summary: "API server returns no config for this binding, with --primary-address",
		args:    []string{"valid-no-config", "--primary-address"},
		code:    1,
		out:     `no network config found for binding "valid-no-config"`,
	}, {
		summary: "API server returns no config for this binding, no --primary-address",
		args:    []string{"valid-no-config"},
		code:    1,
		out:     `no network config found for binding "valid-no-config"`,
	}, {
		summary: "explicitly bound, extra-binding name given with --primary-address",
		args:    []string{"known-extra", "--primary-address"},
		out:     "10.20.1.42",
	}, {
		summary: "explicitly bound, extra-binding name given without --primary-address",
		args:    []string{"known-extra"},
		out: `- macaddress: "00:11:22:33:44:22"
  interfacename: eth2
  addresses:
  - address: 10.20.1.42
    cidr: 10.20.1.42/24
  - address: fc00::1
    cidr: fc00::/64`,
	}, {
		summary: "explicitly bound relation name given with --primary-address",
		args:    []string{"known-relation", "--primary-address"},
		out:     "10.10.0.23",
	}, {
		summary: "explicitly bound relation name given without --primary-address",
		args:    []string{"known-relation"},
		out: `- macaddress: "00:11:22:33:44:00"
  interfacename: eth0
  addresses:
  - address: 10.10.0.23
    cidr: 10.10.0.0/24
  - address: 192.168.1.111
    cidr: 192.168.1.0/24
- macaddress: "00:11:22:33:44:11"
  interfacename: eth1
  addresses:
  - address: 10.10.1.23
    cidr: 10.10.1.0/24
  - address: 192.168.2.111
    cidr: 192.168.2.0/24`,
	}, {
		summary: "implicitly bound binding name given with --primary-address",
		args:    []string{"known-unbound", "--primary-address"},
		out:     "10.33.1.8",
	}, {
		summary: "implicitly bound binding name given without --primary-address",
		args:    []string{"known-unbound"},
		out: `- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - address: 10.33.1.8
    cidr: 10.33.1.8/24`,
	}} {
		c.Logf("test %d: %s", i, t.summary)
		com := s.createCommand(c)
		ctx := cmdtesting.Context(c)
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
			expect := fmt.Sprintf(`(.|\n)*ERROR %s\n`, t.out)
			c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

func (s *NetworkGetSuite) TestHelp(c *gc.C) {

	helpLine := `Usage: network-get [options] <binding-name> --primary-address`

	com := s.createCommand(c)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(strings.Split(bufferString(ctx.Stdout), "\n")[0], gc.Equals, helpLine)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
