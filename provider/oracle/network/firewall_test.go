// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"

	"github.com/juju/go-oracle-cloud/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	jujunetwork "github.com/juju/juju/network"
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

	firewall := network.NewFirewall(cfg, DefaultFakeFirewallAPI)
	c.Assert(firewall, gc.NotNil)

	rule, err := firewall.GlobalIngressRules()
	c.Assert(err, gc.IsNil)
	c.Assert(rule, gc.NotNil)
}

func (u *firewallSuite) TestGlobalIngressRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*FakeFirewallAPI{
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
	} {

		firewall := network.NewFirewall(cfg, fake)
		c.Assert(firewall, gc.NotNil)

		rule, err := firewall.GlobalIngressRules()
		c.Assert(err, gc.NotNil)
		c.Assert(rule, gc.IsNil)
	}

}

func (u *firewallSuite) TestOpenPorts(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwGlobal,
	})
	cfg := &fakeEnvironConfig{cfg: fakeConfig}

	firewall := network.NewFirewall(cfg, DefaultFakeFirewallAPI)
	c.Assert(firewall, gc.NotNil)

	err := firewall.OpenPorts([]jujunetwork.IngressRule{})
	c.Assert(err, gc.IsNil)

}

func (u *firewallSuite) TestOpenPortsWithErrors(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwGlobal,
	})
	cfg := &fakeEnvironConfig{cfg: fakeConfig}

	for _, fake := range []*FakeFirewallAPI{
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: FakeSecList{
				SecListErr: errors.New("FakeSecListErr"),
			},
		},
		&FakeFirewallAPI{
			FakeComposer: FakeComposer{
				compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: FakeSecList{
				SecListErr: api.ErrNotFound{},
				CreateErr:  errors.New("FakeSecListErr"),
			},
		},
	} {
		firewall := network.NewFirewall(cfg, fake)
		c.Assert(firewall, gc.NotNil)

		err := firewall.OpenPorts([]jujunetwork.IngressRule{})
		c.Assert(err, gc.NotNil)
	}

	// test with error in firewall config
	cfg = &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	firewall := network.NewFirewall(cfg, DefaultFakeFirewallAPI)
	c.Assert(firewall, gc.NotNil)

	err := firewall.OpenPorts([]jujunetwork.IngressRule{})
	c.Assert(err, gc.NotNil)
}
