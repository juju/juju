// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	//stdtesting "testing"
	//"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/loggo"
	gc "launchpad.net/gocheck"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	//coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	apinetworker "github.com/juju/juju/state/api/networker"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/networker"
)

var logger = loggo.GetLogger("juju.worker.networker_test")

type networkerSuite struct {
	testing.JujuConnSuite
	defaultConstraints constraints.Value
	cfg *config.Config

	networks []state.NetworkInfo

	machine         *state.Machine
	container       *state.Machine
	nestedContainer *state.Machine

	machineIfaces         []state.NetworkInterfaceInfo
	containerIfaces       []state.NetworkInterfaceInfo
	nestedContainerIfaces []state.NetworkInterfaceInfo

	st        *api.State
	networkerState *apinetworker.State
	wordpress *state.Service
}

var _ = gc.Suite(&networkerSuite{})

var _ worker.StringsWatchHandler = (*networker.Networker)(nil)

// Create several networks.
func (s *networkerSuite) setUpNetworks(c *gc.C) {
	s.networks = []state.NetworkInfo{{
		Name:       "net1",
		ProviderId: "net1",
		CIDR:       "0.1.2.0/24",
		VLANTag:    0,
	}, {
		Name:       "vlan42",
		ProviderId: "vlan42",
		CIDR:       "0.2.2.0/24",
		VLANTag:    42,
	}, {
		Name:       "vlan69",
		ProviderId: "vlan69",
		CIDR:       "0.3.2.0/24",
		VLANTag:    69,
	}, {
		Name:       "vlan123",
		ProviderId: "vlan123",
		CIDR:       "0.4.2.0/24",
		VLANTag:    123,
	}, {
		Name:       "net2",
		ProviderId: "net2",
		CIDR:       "0.5.2.0/24",
		VLANTag:    0,
	}}
}

func (s *networkerSuite) SetUpSuite(c *gc.C) {
        s.JujuConnSuite.SetUpSuite(c)
        s.defaultConstraints = constraints.MustParse("arch=amd64 mem=4G cpu-cores=1 root-disk=8G")

}

// Create a machine and login to it.
func (s *networkerSuite) setUpMachine(c *gc.C) {
	var err error
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	hwChars := instance.MustParseHardware("cpu-cores=123", "mem=4G")
	s.machineIfaces = []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1",
		NetworkName:   "net1",
		IsVirtual:     false,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f1",
		InterfaceName: "eth1.42",
		NetworkName:   "vlan42",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f0",
		InterfaceName: "eth0.69",
		NetworkName:   "vlan69",
		IsVirtual:     true,
	}, {
		MACAddress:    "aa:bb:cc:dd:ee:f2",
		InterfaceName: "eth2",
		NetworkName:   "net2",
		IsVirtual:     false,
	}}
	err = s.machine.SetInstanceInfo("i-am", "fake_nonce", &hwChars, s.networks, s.machineIfaces)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, s.machine.Tag().String(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
}

func (s *networkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.setUpNetworks(c)
	s.setUpMachine(c)

	// Create the networker API facade.
	s.networkerState = s.st.Networker()
	c.Assert(s.networkerState, gc.NotNil)
}

func (s *networkerSuite) TestMachineNetworkInfoPermissionDenied(c *gc.C) {
	tags := []string{"foo-42", "unit-mysql-0", "service-mysql", "user-foo", "machine-1"}
	for _, tag := range tags {
		info, err := s.networkerState.MachineNetworkInfo(tag)
		c.Assert(err, gc.ErrorMatches, "permission denied")
		c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
		c.Assert(info, gc.IsNil)
	}
}

type mockConfig struct {
        agent.Config
        tag string
}

func (mock *mockConfig) Tag() string {
        return mock.tag
}

func agentConfig(tag string) agent.Config {
        return &mockConfig{tag: tag}
}

func (s *networkerSuite) TestNetworker(c *gc.C) {
	// Set up services and units for later use.
	s.wordpress = s.AddTestingServiceWithNetworks(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
		[]string{"net1", "net2"},
	)
	s.wordpress.SetConstraints(constraints.MustParse("networks=vlan42,^net4,^net5"))
	_, err := s.wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	c.Assert(false, gc.IsNil)

	//
	nw := networker.NewNetworker(s.networkerState, agentConfig(s.machine.Tag().String()))
	defer func() { c.Assert(worker.Stop(nw), gc.IsNil) }()

}
