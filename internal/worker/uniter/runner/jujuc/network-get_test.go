// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
)

type NetworkGetSuite struct {
	ContextSuite
}

func TestNetworkGetSuite(t *stdtesting.T) {
	tc.Run(t, &NetworkGetSuite{})
}

func (s *NetworkGetSuite) SetUpSuite(c *tc.C) {
	s.ContextSuite.SetUpSuite(c)
}

func (s *NetworkGetSuite) createCommand(c *tc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")

	presetBindings := make(map[string]params.NetworkInfoResult)
	presetBindings["known-relation"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:00",
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
			{
				MACAddress:    "00:11:22:33:44:11",
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
			{
				MACAddress:    "00:11:22:33:44:22",
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
			{
				MACAddress:    "00:11:22:33:44:33",
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
	// Simulate info with egress and ingress data.
	presetBindings["ingress-egress"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:33",
				InterfaceName: "eth3",
				Addresses: []params.InterfaceAddress{
					{
						Address: "10.33.1.8",
						CIDR:    "10.33.1.8/24",
					},
				},
			},
		},
		IngressAddresses: []string{"100.1.2.3", "100.4.3.2"},
		EgressSubnets:    []string{"192.168.1.0/8", "10.0.0.0/8"},
	}

	hctx.info.NetworkInterface.NetworkInfoResults = presetBindings

	com, err := jujuc.NewCommand(hctx, "network-get")
	c.Assert(err, tc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *NetworkGetSuite) TestNetworkGet(c *tc.C) {
	for i, t := range []struct {
		summary string
		args    []string
		code    int
		out     string
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
		summary: "unknown binding given, no extra args given",
		code:    1,
		args:    []string{"unknown"},
		out:     `no network config found for binding "unknown"`,
	}, {
		summary: "unknown binding given, with additional args",
		args:    []string{"unknown", "--ingress-address"},
		code:    1,
		out:     `no network config found for binding "unknown"`,
	}, {
		summary: "API server returns no config for this binding, with --ingress-address",
		args:    []string{"valid-no-config", "--ingress-address"},
		code:    1,
		out:     `no network config found for binding "valid-no-config"`,
	}, {
		summary: "API server returns no config for this binding, no address args",
		args:    []string{"valid-no-config"},
		code:    1,
		out:     `no network config found for binding "valid-no-config"`,
	}, {
		summary: "explicitly bound, extra-binding name given with single flag arg",
		args:    []string{"known-extra", "--ingress-address"},
		out:     "10.20.1.42",
	}, {
		summary: "explicitly bound, extra-binding name given with multiple flag args",
		args:    []string{"known-extra", "--ingress-address", "--bind-address"},
		out: `
bind-address: 10.20.1.42
ingress-address: 10.20.1.42`[1:],
	}, {
		summary: "explicitly bound, extra-binding name given without extra args",
		args:    []string{"known-extra"},
		out: `
bind-addresses:
- mac-address: "00:11:22:33:44:22"
  interface-name: eth2
  addresses:
  - hostname: ""
    value: 10.20.1.42
    cidr: 10.20.1.42/24
    address: 10.20.1.42
  - hostname: ""
    value: fc00::1
    cidr: fc00::/64
    address: fc00::1
  macaddress: "00:11:22:33:44:22"
  interfacename: eth2`[1:],
	}, {
		summary: "explicitly bound relation name given with single flag arg",
		args:    []string{"known-relation", "--ingress-address"},
		out:     "10.10.0.23",
	}, {
		summary: "explicitly bound relation name given without extra args",
		args:    []string{"known-relation"},
		out: `
bind-addresses:
- mac-address: "00:11:22:33:44:00"
  interface-name: eth0
  addresses:
  - hostname: ""
    value: 10.10.0.23
    cidr: 10.10.0.0/24
    address: 10.10.0.23
  - hostname: ""
    value: 192.168.1.111
    cidr: 192.168.1.0/24
    address: 192.168.1.111
  macaddress: "00:11:22:33:44:00"
  interfacename: eth0
- mac-address: "00:11:22:33:44:11"
  interface-name: eth1
  addresses:
  - hostname: ""
    value: 10.10.1.23
    cidr: 10.10.1.0/24
    address: 10.10.1.23
  - hostname: ""
    value: 192.168.2.111
    cidr: 192.168.2.0/24
    address: 192.168.2.111
  macaddress: "00:11:22:33:44:11"
  interfacename: eth1`[1:],
	}, {
		summary: "no user requested binding falls back to binding address, with ingress-address arg",
		args:    []string{"known-unbound", "--ingress-address"},
		out:     "10.33.1.8",
	}, {
		summary: "no user requested binding falls back to primary address, without address args",
		args:    []string{"known-unbound"},
		out: `
bind-addresses:
- mac-address: "00:11:22:33:44:33"
  interface-name: eth3
  addresses:
  - hostname: ""
    value: 10.33.1.8
    cidr: 10.33.1.8/24
    address: 10.33.1.8
  macaddress: "00:11:22:33:44:33"
  interfacename: eth3`[1:],
	}, {
		summary: "explicit ingress and egress information",
		args:    []string{"ingress-egress", "--ingress-address", "--bind-address", "--egress-subnets"},
		out: `
bind-address: 10.33.1.8
egress-subnets:
- 192.168.1.0/8
- 10.0.0.0/8
ingress-address: 100.1.2.3`[1:],
	}, {
		summary: "explicit ingress and egress information, no extra args",
		args:    []string{"ingress-egress"},
		out: `
bind-addresses:
- mac-address: "00:11:22:33:44:33"
  interface-name: eth3
  addresses:
  - hostname: ""
    value: 10.33.1.8
    cidr: 10.33.1.8/24
    address: 10.33.1.8
  macaddress: "00:11:22:33:44:33"
  interfacename: eth3
egress-subnets:
- 192.168.1.0/8
- 10.0.0.0/8
ingress-addresses:
- 100.1.2.3
- 100.4.3.2`[1:],
	}} {
		c.Logf("test %d: %s", i, t.summary)
		s.testScenario(c, t.args, t.code, t.out)
	}
}

func (s *NetworkGetSuite) testScenario(c *tc.C, args []string, code int, out string) {
	ctx := cmdtesting.Context(c)

	c.Check(cmd.Main(s.createCommand(c), ctx, args), tc.Equals, code)

	if code == 0 {
		c.Check(bufferString(ctx.Stderr), tc.Equals, "")
		expect := out
		if expect != "" {
			expect = expect + "\n"
		}
		c.Check(bufferString(ctx.Stdout), tc.Equals, expect)
	} else {
		c.Check(bufferString(ctx.Stdout), tc.Equals, "")
		expect := fmt.Sprintf(`(.|\n)*ERROR %s\n`, out)
		c.Check(bufferString(ctx.Stderr), tc.Matches, expect)
	}
}
