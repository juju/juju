// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"fmt"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	apinetworker "github.com/juju/juju/state/api/networker"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/networker"
)

type networkerSuite struct {
	testing.JujuConnSuite
	cfg *config.Config

	networks      []state.NetworkInfo
	machine       *state.Machine
	machineIfaces []state.NetworkInterfaceInfo

	st             *api.State
	networkerState *apinetworker.State

	networkDir          string
	interfacesFile      string
	executorCommands    []string
	executorHasFinished chan struct{}
}

var _ = gc.Suite(&networkerSuite{})

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

	// Create test juju state.
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

const sampleInterfacesFile = `# This file describes the network interfaces available on your system
# and how to activate them. For more information see interfaces(5).

# The loopback network interface
auto lo
iface lo inet loopback

auto eth0
source %s/eth0.config

auto eth1
iface eth1 inet dhcp

auto eth1.2
iface eth1.2 inet dhcp

auto eth2
iface eth2 inet dhcp
`
var readyInterfaces = set.NewStrings("eth0", "eth1")
var interfacesWithAddress = set.NewStrings("eth0", "eth2")

const expectedInterfacesFile = `# This file describes the network interfaces available on your system
# and how to activate them. For more information see interfaces(5).

# The loopback network interface
auto lo
iface lo inet loopback

auto eth0
source %s/eth0.config

auto eth1
iface eth1 inet dhcp

auto eth1.2
iface eth1.2 inet dhcp

auto eth2
iface eth2 inet dhcp

auto eth1.42
iface eth1.42 inet dhcp

auto eth0.69
iface eth0.69 inet dhcp
`

func (s *networkerSuite) TestNetworker(c *gc.C) {
	// Create temporary directory to store interfaces file.
	s.networkDir = c.MkDir()
	s.PatchValue(&networker.NetworkDir, s.networkDir)
	s.interfacesFile = s.networkDir + "/interfaces"

	// Create a sample interfaces file
	interfacesFileContents := fmt.Sprintf(sampleInterfacesFile, s.networkDir)
	err := ioutil.WriteFile(s.interfacesFile, []byte(interfacesFileContents), 0644)
	c.Assert(err, gc.IsNil)

	// Patch the network interface functions
	s.PatchValue(&networker.InterfaceIsUp,
		func(name string) bool {
			return readyInterfaces.Contains(name)
		})
	s.PatchValue(&networker.InterfaceHasAddress,
		func(name string) bool {
			return interfacesWithAddress.Contains(name)
		})

	// Path the command executor
	s.PatchValue(&networker.ExecuteCommands,
		func(commands []string) error {
			s.executorCommands = commands
			close(s.executorHasFinished)
			return nil
		})
	s.executorHasFinished = make(chan struct{})

	// Create and setup networker.
	nw := networker.NewNetworker(s.networkerState, agentConfig(s.machine.Tag().String()))
	defer func() { c.Assert(worker.Stop(nw), gc.IsNil) }()

	// Wait until networker setup has finished execute commands.
	<-s.executorHasFinished

	// Verify the contents of generated interfaces file.
	contents, err := ioutil.ReadFile(s.interfacesFile)
	c.Assert(err, gc.IsNil)
	expected := fmt.Sprintf(expectedInterfacesFile, s.networkDir)
	c.Assert(string(contents), gc.Equals, expected)

	// Verify the executed commands
	expectedCommands := []string{
		"dpkg-query -s vlan || apt-get --option Dpkg::Options::=--force-confold --assume-yes install vlan",
		"lsmod | grep -q 8021q || modprobe 8021q",
		"grep -q 8021q /etc/modules || echo 8021q >> /etc/modules",
		"vconfig set_name_type DEV_PLUS_VID_NO_PAD",
		"ifup eth1.42",
		"ifup eth0.69",
		"ifup eth2",
	}
	c.Assert(s.executorCommands, gc.DeepEquals, expectedCommands)
}

func (s *networkerSuite) TestExecuteCommands(c *gc.C) {
	commands := []string{
		"echo start",
		"sh -c 'echo STDOUT; echo STDERR >&2; exit 123'",
		"echo end",
		"exit 111",
	}
	err := networker.ExecuteCommands(commands)
	expected := "command \"sh -c 'echo STDOUT; echo STDERR >&2; exit 123'\" failed " +
		"(code: 123, stdout: STDOUT\n, stderr: STDERR\n)"
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, expected)
}
