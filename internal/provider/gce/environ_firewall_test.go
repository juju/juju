// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/internal/provider/gce"
)

type environFirewallSuite struct {
	gce.BaseSuite
}

func TestEnvironFirewallSuite(t *testing.T) {
	tc.Run(t, &environFirewallSuite{})
}

func (s *environFirewallSuite) generateMachineID() string {
	shortSHA := s.ModelUUID[len(s.ModelUUID)-6:]
	n := rand.Intn(100)
	return fmt.Sprintf("juju-%s-%d", shortSHA, n)
}

func (s *environFirewallSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(gce.FirewallerSuffixFunc, func(sourceCIDRs []string, prefix string, existingNames set.Strings) (string, error) {
		if len(sourceCIDRs) == 0 || len(sourceCIDRs) == 1 &&
			(sourceCIDRs[0] == "0.0.0.0/0" || sourceCIDRs[0] == "::/0") {
			return prefix, nil
		}
		return prefix + "-" + strconv.Itoa(len(strings.Join(sourceCIDRs, ":"))), nil
	})
}

func (s *environFirewallSuite) TestGlobalFirewallName(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	fwPrefix := gce.GlobalFirewallName(env)

	c.Check(fwPrefix, tc.Equals, "juju-"+s.ModelUUID)
}

func (s *environFirewallSuite) TestOpenPortsInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	fwPrefix := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return(nil, gce.InvalidCredentialError)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
	err := env.OpenPorts(c.Context(), gce.GlobalFirewallName(env), rules)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environFirewallSuite) TestOpenPorts(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"81"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix, &computepb.Firewall{
		Name:         ptr(fwPrefix),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"81", "80"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
	err := env.OpenPorts(c.Context(), fwPrefix, rules)
	c.Check(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePorts(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80"},
		}},
	}}, nil)
	s.MockService.EXPECT().RemoveFirewall(gomock.Any(), fwPrefix).Return(nil)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Check(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return(nil, gce.InvalidCredentialError)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80/tcp")),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environFirewallSuite) TestIngressRulesInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return(nil, gce.InvalidCredentialError)

	_, err := env.IngressRules(c.Context(), fwPrefix)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environFirewallSuite) TestIngressRules(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"80-81", "92"},
			}, {
				IPProtocol: ptr("udp"),
				Ports:      []string{"443", "100-120"},
			},
		},
	}}, nil)

	ports, err := env.IngressRules(c.Context(), fwPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("92/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("100-120/udp"), "10.0.0.0/24", "192.168.1.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("443/udp"), "10.0.0.0/24", "192.168.1.0/24"),
		},
	)
}

func (s *environFirewallSuite) TestIngressRulesCollapse(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"81"},
		}, {
			IPProtocol: ptr("tcp"),
			Ports:      []string{"82"},
		}, {
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80"},
		}, {
			IPProtocol: ptr("tcp"),
			Ports:      []string{"83"},
		}, {
			IPProtocol: ptr("tcp"),
			Ports:      []string{"92"},
		}},
	}}, nil)

	ports, err := env.IngressRules(c.Context(), fwPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80-83/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
			firewall.NewIngressRule(network.MustParsePortRange("92/tcp"), "10.0.0.0/24", "192.168.1.0/24"),
		},
	)
}

func (s *environFirewallSuite) TestIngressRulesDefaultCIDR(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:       ptr(fwPrefix),
		TargetTags: []string{fwPrefix},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "92"},
		}},
	}}, nil)

	ports, err := env.IngressRules(c.Context(), fwPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), firewall.AllNetworksIPV4CIDR),
			firewall.NewIngressRule(network.MustParsePortRange("92/tcp"), firewall.AllNetworksIPV4CIDR),
		},
	)
}

func (s *environFirewallSuite) TestOpenPortsAdd(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return(nil, errors.NotFoundf(fwPrefix))
	s.MockService.EXPECT().AddFirewall(gomock.Any(), &computepb.Firewall{
		Name:         ptr(fwPrefix + "-26"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		Network:      ptr("/path/to/vpc"),
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"100-120"},
		}},
	})
	s.MockService.EXPECT().AddFirewall(gomock.Any(), &computepb.Firewall{
		Name:         ptr(fwPrefix + "-11"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		Network:      ptr("/path/to/vpc"),
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("udp"),
			Ports:      []string{"67"},
		}},
	})
	s.MockService.EXPECT().AddFirewall(gomock.Any(), &computepb.Firewall{
		Name:         ptr(fwPrefix),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		Network:      ptr("/path/to/vpc"),
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}, {
			IPProtocol: ptr("udp"),
			Ports:      []string{"80-81"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-81/tcp")), // leave out CIDR to check default
		firewall.NewIngressRule(network.MustParsePortRange("80-81/udp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(network.MustParsePortRange("100-120/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("67/udp"), "10.0.0.0/24"),
	}
	err := env.OpenPorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenPortsUpdateSameCIDR(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24", "10.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
	}
	err := env.OpenPorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenPortsUpdateAddCIDR(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"10.0.0.0/24", "192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "10.0.0.0/24"),
	}
	err := env.OpenPorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenPortsUpdateAndAdd(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}},
	}, {
		Name:         ptr(fwPrefix + "-8e65efabcd"),
		TargetTags:   []string{"spam"},
		SourceRanges: []string{"172.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"100-120", "443"},
		}, {
			IPProtocol: ptr("udp"),
			Ports:      []string{"67"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-8e65efabcd", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-8e65efabcd"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"172.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"100-120", "443"},
		}, {
			IPProtocol: ptr("udp"),
			Ports:      []string{"67"},
		}},
	})
	s.MockService.EXPECT().AddFirewall(gomock.Any(), &computepb.Firewall{
		Name:         ptr(fwPrefix + "-11"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		Network:      ptr("/path/to/vpc"),
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-100", "443"},
		}},
	})
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("80-100/tcp"), "10.0.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "10.0.0.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("67/udp"), "172.0.0.0/24"),
	}
	err := env.OpenPorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsRemove(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"443"},
		}},
	}}, nil)
	s.MockService.EXPECT().RemoveFirewall(gomock.Any(), fwPrefix+"-d01a82")

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp")),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsUpdate(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp")),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsCollapseUpdate(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-80", "100-120", "81-81", "82-82"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"100-120"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("80-82/tcp")),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestClosePortsRemoveCIDR(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24", "10.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	}}, nil)
	s.MockService.EXPECT().UpdateFirewall(gomock.Any(), fwPrefix+"-d01a82", &computepb.Firewall{
		Name:         ptr(fwPrefix + "-d01a82"),
		Description:  ptr("created by Juju with target " + fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"10.0.0.0/24"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	})

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("443/tcp"), "192.168.1.0/24"),
		firewall.NewIngressRule(network.MustParsePortRange("80-81/tcp"), "192.168.1.0/24"),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestCloseNoMatches(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := s.generateMachineID()
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix + "-d01a82"),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"80-81", "443"},
		}},
	}}, nil)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("100-110/tcp"), "192.168.0.1/24"),
	}
	err := env.ClosePorts(c.Context(), fwPrefix, rules)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`closing port(s) [100-110/tcp from 192.168.0.1/24] over non-matching rules not supported`))
}

func (s *environFirewallSuite) TestOpenModelPorts(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{}, nil)
	s.MockService.EXPECT().AddFirewall(
		gomock.Any(),
		&computepb.Firewall{
			Name:         ptr(fwPrefix + "-14"),
			Description:  ptr("created by Juju with target " + fwPrefix),
			TargetTags:   []string{fwPrefix},
			Network:      ptr("/path/to/vpc"),
			SourceRanges: []string{"192.168.1.0/24"},
			Allowed: []*computepb.Allowed{{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			}},
		},
	).Return(nil)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), []string{"192.168.1.0/24"}...),
	}
	err := env.OpenModelPorts(c.Context(), rules)
	c.Check(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestOpenModelPortsInvalidCredentialError(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)

	fwPrefix := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return(nil, gce.InvalidCredentialError)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp")),
	}
	err := env.OpenModelPorts(c.Context(), rules)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environFirewallSuite) TestCloseModelPorts(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"0.0.0.0/0"},
		Allowed: []*computepb.Allowed{{
			IPProtocol: ptr("tcp"),
			Ports:      []string{"22"},
		}},
	}}, nil)
	s.MockService.EXPECT().RemoveFirewall(gomock.Any(), fwPrefix).Return(nil)

	rules := firewall.IngressRules{
		firewall.NewIngressRule(network.MustParsePortRange("22/tcp")),
	}
	err := env.CloseModelPorts(c.Context(), rules)
	c.Check(err, tc.ErrorIsNil)
}

func (s *environFirewallSuite) TestModelIngressRules(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	fwPrefix := "juju-" + s.ModelUUID
	s.MockService.EXPECT().Firewalls(gomock.Any(), fwPrefix).Return([]*computepb.Firewall{{
		Name:         ptr(fwPrefix),
		TargetTags:   []string{fwPrefix},
		SourceRanges: []string{"192.168.1.0/24"},
		Allowed: []*computepb.Allowed{
			{
				IPProtocol: ptr("tcp"),
				Ports:      []string{"22"},
			},
		},
	}}, nil)

	ports, err := env.ModelIngressRules(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		ports, tc.DeepEquals,
		firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), "192.168.1.0/24"),
		},
	)
}
