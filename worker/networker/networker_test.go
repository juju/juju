// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

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
	coretesting "github.com/juju/juju/testing"
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
	configStates   []*configState
	executed       chan bool
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

	// Create temporary directory to store interfaces file.
	s.PatchValue(&networker.ConfigDirName, c.MkDir())
	s.PatchValue(&networker.ConfigFileName, networker.ConfigDirName+"/interfaces")
	s.PatchValue(&networker.ConfigSubDirName, networker.ConfigDirName+"/interfaces.d")

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

auto wlan0
iface wlan0 inet dhcp
`
const sampleEth0DotConfigFile = `iface eth0 inet manual

auto br0
iface br0 inet dhcp
  bridge_ports eth0
`

var readyInterfaces = set.NewStrings("eth0", "br0", "wlan0")
var interfacesWithAddress = set.NewStrings("br0", "wlan0")

var expectedInterfacesFile = `# This file describes the network interfaces available on your system
# and how to activate them. For more information see interfaces(5).

# The loopback network interface
auto lo
iface lo inet loopback

` + networker.SourceCommentAndCommand

type configState struct {
	files    networker.ConfigFiles
	commands []string
	readyInterfaces []string
	interfacesWithAddress []string
}

func executeCommandsHook(c *gc.C, s *networkerSuite, commands []string) error {
	cs := &configState{}
	err := cs.files.ReadAll()
	c.Assert(err, gc.IsNil)
	cs.commands = append(cs.commands, commands...)
	// modify state of interfaces
	for _, cmd := range commands {
		args := strings.Split(cmd, " ")
		if len(args) == 2 && args[0] == "ifup" {
			readyInterfaces.Add(args[1])
			interfacesWithAddress.Add(args[1])
		} else if len(args) == 2 && args[0] == "ifdown" {
			readyInterfaces.Remove(args[1])
			interfacesWithAddress.Remove(args[1])
		}
	}
	cs.readyInterfaces = readyInterfaces.SortedValues()
	cs.interfacesWithAddress = interfacesWithAddress.SortedValues()
	s.configStates = append(s.configStates, cs)
	s.executed <- true
	return nil
}

func (s *networkerSuite) TestNetworker(c *gc.C) {
	// Create a sample interfaces file (MAAS configuration)
	interfacesFileContents := fmt.Sprintf(sampleInterfacesFile, networker.ConfigDirName)
	err := ioutil.WriteFile(networker.ConfigFileName, []byte(interfacesFileContents), 0644)
	c.Assert(err, gc.IsNil)
	err = ioutil.WriteFile(networker.ConfigDirName+"/eth0.config", []byte(sampleEth0DotConfigFile), 0644)
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
	s.configStates = []*configState{}
	s.PatchValue(&networker.ExecuteCommands, func(commands []string) error { return executeCommandsHook(c, s, commands) })

	// Create and setup networker.
	s.executed = make(chan bool)
	nw := networker.NewNetworker(s.networkerState, agentConfig(s.machine.Tag().String()))
	defer func() { c.Assert(worker.Stop(nw), gc.IsNil) }()

	executeCount := 0
loop:
	for {
		select {
		case <-s.executed:
			executeCount++
			if executeCount == 3 {
				break loop
			}
		case <-time.After(coretesting.ShortWait):
			c.Fatalf("command not executed")
		}
	}

	// Verify the executed commands from SetUp()
	expectedConfigFiles := networker.ConfigFiles{
		"": {
			FileName: networker.ConfigFileName,
			Data: fmt.Sprintf(expectedInterfacesFile, networker.ConfigSubDirName,
				networker.ConfigSubDirName, networker.ConfigSubDirName, networker.ConfigSubDirName),
		},
		"br0": {
			FileName: networker.FileName("br0"),
			Data: "auto br0\niface br0 inet dhcp\n  bridge_ports eth0\n",
		},
		"eth0": {
			FileName: networker.FileName("eth0"),
			Data: "auto eth0\niface eth0 inet manual\n",
		},
		"wlan0": {
			FileName: networker.FileName("wlan0"),
			Data: "auto wlan0\niface wlan0 inet dhcp\n",
		},
	}
	c.Assert(s.configStates[0].files, gc.DeepEquals, expectedConfigFiles)
	expectedCommands := []string{
		"dpkg-query -s vlan || apt-get --option Dpkg::Options::=--force-confold --assume-yes install vlan",
		"lsmod | grep -q 8021q || modprobe 8021q",
		"grep -q 8021q /etc/modules || echo 8021q >> /etc/modules",
		"vconfig set_name_type DEV_PLUS_VID_NO_PAD",
	}
	c.Assert(s.configStates[0].commands, gc.DeepEquals, expectedCommands)
	c.Assert(s.configStates[0].readyInterfaces, gc.DeepEquals, []string{"br0", "eth0", "wlan0"})
	c.Assert(s.configStates[0].interfacesWithAddress, gc.DeepEquals, []string{"br0", "wlan0"})

	// Verify the executed commands from Handle()
	c.Assert(s.configStates[1].files, gc.DeepEquals, expectedConfigFiles)
	expectedCommands = []string(nil)
	c.Assert(s.configStates[1].commands, gc.DeepEquals, expectedCommands)
	c.Assert(s.configStates[1].readyInterfaces, gc.DeepEquals, []string{"br0", "eth0", "wlan0"})
	c.Assert(s.configStates[1].interfacesWithAddress, gc.DeepEquals, []string{"br0", "wlan0"})

	// Verify the executed commands from Handle()
	expectedConfigFiles["eth0.69"] = &networker.ConfigFile{
		FileName: networker.FileName("eth0.69"),
		Data: "# Managed by Networker, don't change\nauto eth0.69\niface eth0.69 inet dhcp\n\n",
	}
	expectedConfigFiles["eth1"] = &networker.ConfigFile{
		FileName: networker.FileName("eth1"),
		Data: "# Managed by Networker, don't change\nauto eth1\niface eth1 inet dhcp\n\n",
	}
	expectedConfigFiles["eth1.42"] = &networker.ConfigFile{
		FileName: networker.FileName("eth1.42"),
		Data: "# Managed by Networker, don't change\nauto eth1.42\niface eth1.42 inet dhcp\n\n",
	}
	expectedConfigFiles["eth2"] = &networker.ConfigFile{
		FileName: networker.FileName("eth2"),
		Data: "# Managed by Networker, don't change\nauto eth2\niface eth2 inet dhcp\n\n",
	}
	for k, _ := range s.configStates[2].files {
		c.Check(s.configStates[2].files[k], gc.DeepEquals, expectedConfigFiles[k])
	}
	c.Assert(s.configStates[2].files, gc.DeepEquals, expectedConfigFiles)
	expectedCommands = []string{"ifup eth0.69", "ifup eth1", "ifup eth1.42", "ifup eth2"}
	c.Assert(s.configStates[2].commands, gc.DeepEquals, expectedCommands)
	c.Assert(s.configStates[2].readyInterfaces, gc.DeepEquals,
		[]string{"br0", "eth0", "eth0.69", "eth1", "eth1.42", "eth2", "wlan0"})
	c.Assert(s.configStates[2].interfacesWithAddress, gc.DeepEquals,
		[]string{"br0", "eth0.69", "eth1", "eth1.42", "eth2", "wlan0"})
}
