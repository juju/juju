// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	oraclenetwork "github.com/juju/juju/provider/oracle/network"
	providertest "github.com/juju/juju/provider/oracle/testing"
	"github.com/juju/juju/testing"
)

type firewallSuite struct {
	gitjujutesting.IsolationSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&firewallSuite{})

func (f *firewallSuite) SetUpTest(c *gc.C) {
	f.IsolationSuite.SetUpTest(c)
	f.callCtx = context.NewCloudCallContext()
}

var clk = testclock.NewClock(time.Time{})
var advancingClock = testclock.AutoAdvancingClock{clk, clk.Advance}

type fakeEnvironConfig struct {
	cfg *config.Config
}

func (f *fakeEnvironConfig) Config() *config.Config {
	return f.cfg
}

func (f *firewallSuite) TestNewFirewall(c *gc.C) {
	firewall := oraclenetwork.NewFirewall(nil, nil, &advancingClock)
	c.Assert(firewall, gc.NotNil)

	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	cli := &api.Client{}
	firewall = oraclenetwork.NewFirewall(cfg, cli, &advancingClock)
	c.Assert(firewall, gc.NotNil)
}

func (f *firewallSuite) TestGlobalIngressRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	firewall := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(firewall, gc.NotNil)

	rule, err := firewall.GlobalIngressRules(f.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(rule, gc.NotNil)
}

func (f *firewallSuite) TestIngressRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	firewall := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(firewall, gc.NotNil)

	rule, err := firewall.IngressRules(f.callCtx)
	c.Assert(err, gc.IsNil)
	c.Assert(rule, gc.NotNil)
}

func (f *firewallSuite) TestIngressRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				AllErr: errors.New("FakeRulesError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
	} {

		firewall := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(firewall, gc.NotNil)

		rule, err := firewall.IngressRules(f.callCtx)
		c.Assert(err, gc.NotNil)
		c.Assert(rule, gc.IsNil)
	}

}
func (f *firewallSuite) TestGlobalIngressRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
	} {

		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		rule, err := fw.GlobalIngressRules(f.callCtx)
		c.Assert(err, gc.NotNil)
		c.Assert(rule, gc.IsNil)
	}

}

func (f *firewallSuite) TestOpenPorts(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwGlobal,
	})
	cfg := &fakeEnvironConfig{cfg: fakeConfig}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	err := fw.OpenPorts(f.callCtx, firewall.IngressRules{})
	c.Assert(err, gc.IsNil)

}

func (f *firewallSuite) TestOpenPortsWithErrors(c *gc.C) {
	fakeConfig := testing.CustomModelConfig(c, testing.Attrs{
		"firewall-mode": config.FwGlobal,
	})
	cfg := &fakeEnvironConfig{cfg: fakeConfig}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: errors.New("FakeSecListErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: api.ErrNotFound{},
				CreateErr:  errors.New("FakeSecListErr"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.OpenPorts(f.callCtx, firewall.IngressRules{})
		c.Assert(err, gc.NotNil)
	}

	// test with error in firewall config
	cfg = &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	err := fw.OpenPorts(f.callCtx, firewall.IngressRules{})
	c.Assert(err, gc.NotNil)
}

func (f *firewallSuite) TestClosePorts(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	err := fw.ClosePorts(f.callCtx, firewall.IngressRules{})
	c.Assert(err, gc.IsNil)
}

func (f *firewallSuite) TestClosePortsWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				AllErr: errors.New("FakeRulesErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{
				AllErr: errors.New("FakeSecIpErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{
				AllDefaultErr: errors.New("FakeSecIpErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				All: response.AllSecRules{
					Result: []response.SecRule{
						{
							Action:      common.SecRulePermit,
							Application: "/Compute-acme/jack.jones@example.com/video_streaming_udp",
							Name:        "/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Dst_list:    "seclist:/Compute-acme/jack.jones@example.com/allowed_video_servers",
							Src_list:    "seciplist:/Compute-acme/jack.jones@example.com/es_iplist",
							Uri:         "https://api-z999.compute.us0.oraclecloud.com/secrule/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Src_is_ip:   "true",
							Dst_is_ip:   "false",
						},
					},
				},
				AllErr:    nil,
				DeleteErr: errors.New("FakeSecRules"),
			},
			FakeApplication: providertest.FakeApplication{
				All: response.AllSecApplications{
					Result: []response.SecApplication{
						{
							Description: "Juju created security application",
							Dport:       "17070",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
							Value1:      17070,
							Value2:      -1,
							Id:          "1869cb17-5b12-49c5-a09a-046da8899bc9",
						},
						{
							Description: "Juju created security application",
							Dport:       "37017",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
							Value1:      37017,
							Value2:      -1,
							Id:          "cbefdac0-7684-4f81-a575-825c175aa7b4",
						},
					},
				},
				AllErr: nil,
				Default: response.AllSecApplications{
					Result: []response.SecApplication{
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/all",
							Protocol:    "all",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/all",
							Value1:      0,
							Value2:      0,
							Id:          "381c2267-1b38-4bbd-b53d-5149deddb094",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "echo",
							Name:        "/oracle/public/pings",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/pings",
							Value1:      8,
							Value2:      0,
							Id:          "57b0350b-2f02-4a2d-b5ec-cf731de36027",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/icmp",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/icmp",
							Value1:      255,
							Value2:      255,
							Id:          "abb27ccd-1872-48f9-86ef-38c72d6f8a38",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "reply",
							Name:        "/oracle/public/ping-reply",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ping-reply",
							Value1:      0,
							Value2:      0,
							Id:          "3ad808d4-b740-42c1-805c-57feb7c96d40",
						},
						{
							Description: "",
							Dport:       "3306",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/mysql",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/mysql",
							Value1:      3306,
							Value2:      -1,
							Id:          "2fb5eaff-3127-4334-8b03-367a44bb83bd",
						},
						{
							Description: "",
							Dport:       "22",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/ssh",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ssh",
							Value1:      22, Value2: -1,
							Id: "5f027043-f6b3-4e1a-b9fa-a10d075744de",
						},
					},
				},
				DefaultErr: nil,
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.ClosePorts(f.callCtx, firewall.IngressRules{
			{
				PortRange: network.PortRange{
					FromPort: 0,
					ToPort:   0,
				},
				SourceCIDRs: nil,
			},
		})
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestClosePortsOnInstance(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				AllErr: errors.New("FakeRulesErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{
				AllErr: errors.New("FakeSecIpErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{
				AllDefaultErr: errors.New("FakeSecIpErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				All: response.AllSecRules{
					Result: []response.SecRule{
						{
							Action:      common.SecRulePermit,
							Application: "/Compute-acme/jack.jones@example.com/video_streaming_udp",
							Name:        "/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Dst_list:    "seclist:/Compute-acme/jack.jones@example.com/allowed_video_servers",
							Src_list:    "seciplist:/Compute-acme/jack.jones@example.com/es_iplist",
							Uri:         "https://api-z999.compute.us0.oraclecloud.com/secrule/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Src_is_ip:   "true",
							Dst_is_ip:   "false",
						},
					},
				},
				AllErr:    nil,
				DeleteErr: errors.New("FakeSecRules"),
			},
			FakeApplication: providertest.FakeApplication{
				All: response.AllSecApplications{
					Result: []response.SecApplication{
						{
							Description: "Juju created security application",
							Dport:       "17070",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-7993630e-d13b-43a3-850e-a1778c7e394e",
							Value1:      17070,
							Value2:      -1,
							Id:          "1869cb17-5b12-49c5-a09a-046da8899bc9",
						},
						{
							Description: "Juju created security application",
							Dport:       "37017",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/Compute-a432100/sgiulitti@cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/Compute-a432100/sgiulitti%40cloudbase.com/juju-72324bcb-e837-4542-8867-844282af22e3-ef8a7955-4315-47a2-83c1-8d2978ab77c7",
							Value1:      37017,
							Value2:      -1,
							Id:          "cbefdac0-7684-4f81-a575-825c175aa7b4",
						},
					},
				},
				AllErr: nil,
				Default: response.AllSecApplications{
					Result: []response.SecApplication{
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/all",
							Protocol:    "all",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/all",
							Value1:      0,
							Value2:      0,
							Id:          "381c2267-1b38-4bbd-b53d-5149deddb094",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "echo",
							Name:        "/oracle/public/pings",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/pings",
							Value1:      8,
							Value2:      0,
							Id:          "57b0350b-2f02-4a2d-b5ec-cf731de36027",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/icmp",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/icmp",
							Value1:      255,
							Value2:      255,
							Id:          "abb27ccd-1872-48f9-86ef-38c72d6f8a38",
						},
						{
							Description: "",
							Dport:       "",
							Icmpcode:    "",
							Icmptype:    "reply",
							Name:        "/oracle/public/ping-reply",
							Protocol:    "icmp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ping-reply",
							Value1:      0,
							Value2:      0,
							Id:          "3ad808d4-b740-42c1-805c-57feb7c96d40",
						},
						{
							Description: "",
							Dport:       "3306",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/mysql",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/mysql",
							Value1:      3306,
							Value2:      -1,
							Id:          "2fb5eaff-3127-4334-8b03-367a44bb83bd",
						},
						{
							Description: "",
							Dport:       "22",
							Icmpcode:    "",
							Icmptype:    "",
							Name:        "/oracle/public/ssh",
							Protocol:    "tcp",
							Uri:         "https://compute.uscom-central-1.oraclecloud.com/secapplication/oracle/public/ssh",
							Value1:      22, Value2: -1,
							Id: "5f027043-f6b3-4e1a-b9fa-a10d075744de",
						},
					},
				},
				DefaultErr: nil,
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.ClosePortsOnInstance(f.callCtx, "0 ,", firewall.IngressRules{
			{
				PortRange: network.PortRange{
					FromPort: 0,
					ToPort:   0,
				},
				SourceCIDRs: nil,
			},
		})
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestMachineIngressRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	rules, err := fw.MachineIngressRules(f.callCtx, "0")
	c.Assert(err, gc.IsNil)
	c.Assert(rules, gc.NotNil)
}

func (f *firewallSuite) TestMachineIngressRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{
				AllDefaultErr: errors.New("FakeSecIpError"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		_, err := fw.MachineIngressRules(f.callCtx, "0")
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestOpenPortsOnInstance(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	err := fw.OpenPortsOnInstance(f.callCtx, "0", firewall.IngressRules{})
	c.Assert(err, gc.IsNil)

}

func (f *firewallSuite) TestOpenPortsOnInstanceWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: errors.New("FakeSecListErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: api.ErrNotFound{},
				CreateErr:  errors.New("FakeSecListErr"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.OpenPortsOnInstance(f.callCtx, "0", firewall.IngressRules{})
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestCreateMachineSecLists(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	lists, err := fw.CreateMachineSecLists("0", 7070)
	c.Assert(err, gc.IsNil)
	c.Assert(lists, gc.NotNil)
}

func (f *firewallSuite) TestCreateMachineSecListsWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: errors.New("FakeSecListErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				SecListErr: api.ErrNotFound{},
				CreateErr:  errors.New("FakeSecListErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				AllErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeApplication: providertest.FakeApplication{
				DefaultErr: errors.New("FakeApplicationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecIp: providertest.FakeSecIp{AllErr: errors.New("FakeSecIpError")},
		},
	} {

		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		_, err := fw.CreateMachineSecLists("0", 7070)
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestDeleteMachineSecList(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	err := fw.DeleteMachineSecList("0")
	c.Assert(err, gc.IsNil)
}

func (f *firewallSuite) TestDeleteMachineSecListWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeAssociation: providertest.FakeAssociation{
				AllErr: errors.New("FakeAssociationError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{AllErr: errors.New("FakeRulesError")},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeRules: providertest.FakeRules{
				All: response.AllSecRules{
					Result: []response.SecRule{
						{
							Action:      common.SecRulePermit,
							Application: "/Compute-acme/jack.jones@example.com/video_streaming_udp",
							Name:        "/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Dst_list:    "seclist:/Compute-acme/jack.jones@example.com/allowed_video_servers",
							Src_list:    "seciplist:/Compute-acme/jack.jones@example.com/es_iplist",
							Uri:         "https://api-z999.compute.us0.oraclecloud.com/secrule/Compute-acme/jack.jones@example.com/es_to_videoservers_stream",
							Src_is_ip:   "true",
							Dst_is_ip:   "false",
						},
					},
				},

				DeleteErr: errors.New("FakeRulesError"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecList: providertest.FakeSecList{
				DeleteErr: errors.New("FakeSecListErr"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.DeleteMachineSecList("0")
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestCreateDefaultACLAndRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)

	acls, err := fw.CreateDefaultACLAndRules("0")
	c.Assert(err, gc.IsNil)
	c.Assert(acls, gc.NotNil)
}

func (f *firewallSuite) TestCreateDefaultACLAndRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeAcl: providertest.FakeAcl{
				AclErr: errors.New("FakeAclErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeAcl: providertest.FakeAcl{
				AclErr:    api.ErrNotFound{},
				CreateErr: errors.New("FakeAclErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecRules: providertest.FakeSecRules{
				AllErr: errors.New("FakeAclErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecRules: providertest.FakeSecRules{
				CreateErr: errors.New("FakeAclErr"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		_, err := fw.CreateDefaultACLAndRules("0")
		c.Assert(err, gc.NotNil)
	}
}

func (f *firewallSuite) TestRemoveACLAndRules(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}

	fw := oraclenetwork.NewFirewall(cfg, providertest.DefaultFakeFirewallAPI, &advancingClock)
	c.Assert(fw, gc.NotNil)
	err := fw.RemoveACLAndRules("0")
	c.Assert(err, gc.IsNil)
}

func (f *firewallSuite) TestRemoveACLAndRulesWithErrors(c *gc.C) {
	cfg := &fakeEnvironConfig{cfg: testing.ModelConfig(c)}
	for _, fake := range []*providertest.FakeFirewallAPI{
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecRules: providertest.FakeSecRules{
				AllErr: errors.New("FakeSecRulesErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeSecRules: providertest.FakeSecRules{
				All: response.AllSecurityRules{
					Result: []response.SecurityRule{
						{
							Name:                   "/Compute-acme/jack.jones@example.com/allowed_video_servers",
							Uri:                    "https://api-z999.compute.us0.oraclecloud.com:443/network/v1/secrule/Compute-acme/jack.jones@example.com/secrule1",
							Description:            "Sample security rule",
							Tags:                   nil,
							Acl:                    "/Compute-acme/jack.jones@example.com/allowed_video_servers",
							FlowDirection:          common.Egress,
							SrcVnicSet:             "/Compute-acme/jack.jones@example.com/vnicset1",
							DstVnicSet:             "/Compute-acme/jack.jones@example.com/vnicset2",
							SrcIpAddressPrefixSets: []string{"/Compute-acme/jack.jones@example.com/ipaddressprefixset1"},
							DstIpAddressPrefixSets: nil,
							SecProtocols:           []string{"/Compute-acme/jack.jones@example.com/secprotocol1"},
							EnabledFlag:            true,
						},
					},
				},
				AllErr:    nil,
				DeleteErr: errors.New("FakeSecRulesErr"),
			},
		},
		{
			FakeComposer: providertest.FakeComposer{
				Compose: "/Compute-acme/jack.jones@example.com/allowed_video_servers",
			},
			FakeAcl: providertest.FakeAcl{
				DeleteErr: errors.New("FakeAclErr"),
			},
		},
	} {
		fw := oraclenetwork.NewFirewall(cfg, fake, &advancingClock)
		c.Assert(fw, gc.NotNil)

		err := fw.RemoveACLAndRules("0")
		c.Assert(err, gc.NotNil)
	}
}
