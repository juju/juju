// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"fmt"

	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/oracle/network"
	"github.com/juju/juju/testing"
)

type firewallSuite struct{}

var _ = gc.Suite(&firewallSuite{})

type fakeEnvironConfig struct {
	cfg *config.Config
}

func (f *fakeEnvironConfig) Config() *config.Config {
	return f.cfg
}

func (u *firewallSuite) TestNewFirewall(c *gc.C) {
	firewall := network.NewFirewall(nil, nil)
	c.Assert(firewall, gc.NotNil)

	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	cli := &api.Client{}
	firewall = network.NewFirewall(cfg, cli)
	c.Assert(firewall, gc.NotNil)
}

func (u *firewallSuite) TestGlobalIngressRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	cli := &FakeFirewallAPI{
		FakeComposer: FakeComposer{
			compose: "some-test-string",
		},
		FakeRules: FakeRules{
			All: response.AllSecRules{
				Result: []response.SecRule{
					response.SecRule{
						Action:      common.SecRulePermit,
						Application: "/Compute-acme/jack.jones@example.com/video_streaming_udp",
						Name:        "/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
						Dst_list:    "seclist:/Compute-acme/jack.jones@example.com/allowed_video_servers",
						Src_list:    "seciplist:/Compute-acme/jack.jones@example.com/es_iplist",
						Uri:         "https://api-z999.compute.us0.oraclecloud.com/secrule/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
					},
				},
			},
		},
		//TODO
	}

	firewall := network.NewFirewall(cfg, cli)
	c.Assert(firewall, gc.NotNil)

	rule, err := firewall.GlobalIngressRules()
	c.Assert(err, gc.IsNil)
	fmt.Println(rule)
}
