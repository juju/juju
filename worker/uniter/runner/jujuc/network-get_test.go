// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type NetworkGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&NetworkGetSuite{})

func (s *NetworkGetSuite) SetUpSuite(c *gc.C) {
	s.ContextSuite.SetUpSuite(c)
	lookupHost := func(host string) (addrs []string, err error) {
		return []string{"127.0.1.1", "10.3.3.3"}, nil
	}
	testing.PatchValue(&jujuc.LookupHost, lookupHost)
}

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
	// Simulate info with egress and ingress data.
	presetBindings["ingress-egress"] = params.NetworkInfoResult{
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
		IngressAddresses: []string{"100.1.2.3", "100.4.3.2"},
		EgressSubnets:    []string{"192.168.1.0/8", "10.0.0.0/8"},
	}

	// This should not happen. A hostname should never populate the address
	// field. However, until the code is updated to prevent addresses from
	// being populated by Hostnames (e.g. Change the Address field type from
	// `string` to `net.IP` which will ensure all code paths exclude string
	// population) we will have this check in place to ensure that hostnames
	// are resolved to IPs and that hostnames populate a distinct field.
	// `network-get --primary-address` and the like should only ever return
	// IPs.
	presetBindings["resolvable-hostname"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{MACAddress: "00:11:22:33:44:33",
				InterfaceName: "eth3",
				Addresses: []params.InterfaceAddress{
					{
						Address: "resolvable-hostname",
						CIDR:    "10.33.1.8/24",
					},
				},
			},
		},
		IngressAddresses: []string{"resolvable-hostname"},
	}

	hctx.info.NetworkInterface.NetworkInfoResults = presetBindings

	com, err := jujuc.NewCommand(hctx, cmdString("network-get"))
	c.Assert(err, jc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *NetworkGetSuite) TestNetworkGet(c *gc.C) {
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
- macaddress: "00:11:22:33:44:22"
  interfacename: eth2
  addresses:
  - hostname: ""
    address: 10.20.1.42
    cidr: 10.20.1.42/24
  - hostname: ""
    address: fc00::1
    cidr: fc00::/64`[1:],
	}, {
		summary: "explicitly bound relation name given with single flag arg",
		args:    []string{"known-relation", "--ingress-address"},
		out:     "10.10.0.23",
	}, {
		summary: "explicitly bound relation name given without extra args",
		args:    []string{"known-relation"},
		out: `
bind-addresses:
- macaddress: "00:11:22:33:44:00"
  interfacename: eth0
  addresses:
  - hostname: ""
    address: 10.10.0.23
    cidr: 10.10.0.0/24
  - hostname: ""
    address: 192.168.1.111
    cidr: 192.168.1.0/24
- macaddress: "00:11:22:33:44:11"
  interfacename: eth1
  addresses:
  - hostname: ""
    address: 10.10.1.23
    cidr: 10.10.1.0/24
  - hostname: ""
    address: 192.168.2.111
    cidr: 192.168.2.0/24`[1:],
	}, {
		summary: "no user requested binding falls back to binding address, with ingress-address arg",
		args:    []string{"known-unbound", "--ingress-address"},
		out:     "10.33.1.8",
	}, {
		summary: "no user requested binding falls back to primary address, without address args",
		args:    []string{"known-unbound"},
		out: `
bind-addresses:
- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - hostname: ""
    address: 10.33.1.8
    cidr: 10.33.1.8/24`[1:],
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
- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - hostname: ""
    address: 10.33.1.8
    cidr: 10.33.1.8/24
egress-subnets:
- 192.168.1.0/8
- 10.0.0.0/8
ingress-addresses:
- 100.1.2.3
- 100.4.3.2`[1:],
	}, {
		summary: "a resolvable hostname as address, no args",
		args:    []string{"resolvable-hostname"},
		out: `
bind-addresses:
- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - hostname: resolvable-hostname
    address: 10.3.3.3
    cidr: 10.33.1.8/24
ingress-addresses:
- 10.3.3.3`[1:],
	}} {
		c.Logf("test %d: %s", i, t.summary)
		s.testScenario(c, t.args, t.code, t.out)
	}
}

func (s *NetworkGetSuite) TestNetworkGetLoopbackOnly(c *gc.C) {
	lookupHost := func(host string) (addrs []string, err error) {
		return []string{"127.0.1.1"}, nil
	}
	testing.PatchValue(&jujuc.LookupHost, lookupHost)

	s.testScenario(c, []string{"resolvable-hostname"}, 0, `
bind-addresses:
- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - hostname: resolvable-hostname
    address: ""
    cidr: 10.33.1.8/24`[1:])
}

func (s *NetworkGetSuite) TestNetworkGetDoNotResolve(c *gc.C) {
	s.testScenario(c, []string{"resolvable-hostname", "--resolve-ingress-addresses=false"}, 0, `
bind-addresses:
- macaddress: "00:11:22:33:44:33"
  interfacename: eth3
  addresses:
  - hostname: resolvable-hostname
    address: 10.3.3.3
    cidr: 10.33.1.8/24
ingress-addresses:
- resolvable-hostname`[1:])
}

func (s *NetworkGetSuite) testScenario(c *gc.C, args []string, code int, out string) {
	ctx := cmdtesting.Context(c)

	c.Check(cmd.Main(s.createCommand(c), ctx, args), gc.Equals, code)

	if code == 0 {
		c.Check(bufferString(ctx.Stderr), gc.Equals, "")
		expect := out
		if expect != "" {
			expect = expect + "\n"
		}
		c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
	} else {
		c.Check(bufferString(ctx.Stdout), gc.Equals, "")
		expect := fmt.Sprintf(`(.|\n)*ERROR %s\n`, out)
		c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
	}
}

func (s *NetworkGetSuite) TestHelp(c *gc.C) {
	helpLine := `Usage: network-get [options] <binding-name> [--ingress-address] [--bind-address] [--egress-subnets]`

	com := s.createCommand(c)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)

	c.Check(strings.Split(bufferString(ctx.Stdout), "\n")[0], gc.Equals, helpLine)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "")
}
