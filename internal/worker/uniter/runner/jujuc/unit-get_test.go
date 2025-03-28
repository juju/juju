// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
)

type UnitGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&UnitGetSuite{})

var unitGetTests = []struct {
	args []string
	out  string
}{
	{[]string{"private-address"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "yaml"}, "192.168.0.99\n"},
	{[]string{"private-address", "--format", "json"}, `"192.168.0.99"` + "\n"},
	{[]string{"public-address"}, "gimli.minecraft.testing.invalid\n"},
	{[]string{"public-address", "--format", "yaml"}, "gimli.minecraft.testing.invalid\n"},
	{[]string{"public-address", "--format", "json"}, `"gimli.minecraft.testing.invalid"` + "\n"},
}

func (s *UnitGetSuite) createCommand(c *gc.C) cmd.Command {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewHookCommand(hctx, "unit-get")
	c.Assert(err, jc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(com)
}

func (s *UnitGetSuite) TestOutputFormat(c *gc.C) {
	for _, t := range unitGetTests {
		com := s.createCommand(c)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Check(code, gc.Equals, 0)
		c.Check(bufferString(ctx.Stderr), gc.Equals, "")
		c.Check(bufferString(ctx.Stdout), gc.Matches, t.out)
	}
}

func (s *UnitGetSuite) TestOutputPath(c *gc.C) {
	com := s.createCommand(c)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "private-address"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
	content, err := os.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, "192.168.0.99\n")
}

func (s *UnitGetSuite) TestUnknownSetting(c *gc.C) {
	com := s.createCommand(c)
	err := cmdtesting.InitCommand(com, []string{"protected-address"})
	c.Assert(err, gc.ErrorMatches, `unknown setting "protected-address"`)
}

func (s *UnitGetSuite) TestUnknownArg(c *gc.C) {
	com := s.createCommand(c)
	err := cmdtesting.InitCommand(com, []string{"private-address", "blah"})
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["blah"\]`)
}

func (s *UnitGetSuite) TestNetworkInfoPrivateAddress(c *gc.C) {

	// first - test with no NetworkInfoResults, should fall back
	resultsEmpty := make(map[string]params.NetworkInfoResult)
	resultsNoDefault := make(map[string]params.NetworkInfoResult)
	resultsNoDefault["binding"] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:0",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{

						Address: "10.20.1.42",
						CIDR:    "10.20.1.42/24",
					},
				},
			},
		},
	}
	resultsDefaultNoAddress := make(map[string]params.NetworkInfoResult)
	resultsDefaultNoAddress[""] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:0",
				InterfaceName: "eth0",
			},
		},
	}
	resultsDefaultAddress := make(map[string]params.NetworkInfoResult)
	resultsDefaultAddress[""] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:0",
				InterfaceName: "eth0",
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

	resultsDefaultAddressV6 := make(map[string]params.NetworkInfoResult)
	resultsDefaultAddressV6[""] = params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:44:0",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{
						Address: "fc00::1",
						CIDR:    "fc00::/64",
					},
				},
			},
		},
	}

	resultsResolvedHost := map[string]params.NetworkInfoResult{
		"": {
			Info: []params.NetworkInfo{{
				MACAddress:    "00:11:22:33:44:0",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{
						Address:  "10.20.1.42",
						Hostname: "host-name.somewhere.invalid",
					},
				},
			}},
		},
	}

	launchCommand := func(input map[string]params.NetworkInfoResult, expected string) {
		hctx := s.GetHookContext(c, -1, "")
		hctx.info.NetworkInterface.NetworkInfoResults = input
		com, err := jujuc.NewHookCommand(hctx, "unit-get")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"private-address"})
		c.Assert(code, gc.Equals, 0)
		c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
		c.Assert(bufferString(ctx.Stdout), gc.Equals, expected+"\n")
	}

	launchCommand(resultsEmpty, "192.168.0.99")
	launchCommand(resultsNoDefault, "192.168.0.99")
	launchCommand(resultsDefaultNoAddress, "192.168.0.99")
	launchCommand(resultsDefaultAddress, "10.20.1.42")
	launchCommand(resultsDefaultAddressV6, "fc00::1")
	launchCommand(resultsResolvedHost, "host-name.somewhere.invalid")
}
