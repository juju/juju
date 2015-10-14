// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker_test

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apinetworker "github.com/juju/juju/api/networker"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/networker"
)

type networkerSuite struct {
	testing.JujuConnSuite

	stateMachine    *state.Machine
	stateNetworks   []state.NetworkInfo
	stateInterfaces []state.NetworkInterfaceInfo

	upInterfaces          set.Strings
	interfacesWithAddress set.Strings
	machineInterfaces     []net.Interface
	vlanModuleLoaded      bool
	lastCommands          chan []string

	apiState  api.Connection
	apiFacade apinetworker.State
}

var _ = gc.Suite(&networkerSuite{})

func (s *networkerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// Setup testing state.
	s.setUpNetworks(c)
	s.setUpMachine(c)

	s.machineInterfaces = []net.Interface{
		{Index: 1, MTU: 65535, Name: "lo", Flags: net.FlagUp | net.FlagLoopback},
		{Index: 2, MTU: 1500, Name: "eth0", Flags: net.FlagUp},
		{Index: 3, MTU: 1500, Name: "eth1"},
		{Index: 4, MTU: 1500, Name: "eth2"},
	}
	s.PatchValue(&networker.InterfaceIsUp, func(name string) bool {
		return s.upInterfaces.Contains(name)
	})
	s.PatchValue(&networker.InterfaceHasAddress, func(name string) bool {
		return s.interfacesWithAddress.Contains(name)
	})
	s.PatchValue(&networker.ExecuteCommands, func(commands []string) error {
		return s.executeCommandsHook(c, commands)
	})
	s.PatchValue(&networker.Interfaces, func() ([]net.Interface, error) {
		return s.machineInterfaces, nil
	})

	// Create the networker API facade.
	s.apiFacade = s.apiState.Networker()
	c.Assert(s.apiFacade, gc.NotNil)
}

func (s *networkerSuite) TestStartStop(c *gc.C) {
	nw := s.newNetworker(c, true)
	c.Assert(worker.Stop(nw), gc.IsNil)
}

func (s *networkerSuite) TestConfigPaths(c *gc.C) {
	nw, configDir := s.newCustomNetworker(c, s.apiFacade, s.stateMachine.Id(), true, true)
	defer worker.Stop(nw)

	c.Assert(nw.ConfigBaseDir(), gc.Equals, configDir)
	subdir := filepath.Join(configDir, "interfaces.d")
	c.Assert(nw.ConfigSubDir(), gc.Equals, subdir)
	c.Assert(nw.ConfigFile(""), gc.Equals, filepath.Join(configDir, "interfaces"))
	c.Assert(nw.ConfigFile("ethX.42"), gc.Equals, filepath.Join(subdir, "ethX.42.cfg"))
}

func (s *networkerSuite) TestSafeNetworkerCannotWriteConfig(c *gc.C) {
	c.Skip("enable once the networker is enabled again")

	nw := s.newNetworker(c, false)
	defer worker.Stop(nw)
	c.Assert(nw.IntrusiveMode(), jc.IsFalse)

	select {
	case cmds := <-s.lastCommands:
		c.Fatalf("no commands expected, got %v", cmds)
	case <-time.After(coretesting.ShortWait):
		s.assertNoConfig(c, nw, "", "lo", "eth0", "eth1", "eth1.42", "eth0.69")
	}
}

func (s *networkerSuite) TestNormalNetworkerCanWriteConfigAndLoadsVLANModule(c *gc.C) {
	c.Skip("enable once the networker is enabled again")

	nw := s.newNetworker(c, true)
	defer worker.Stop(nw)
	c.Assert(nw.IntrusiveMode(), jc.IsTrue)

	select {
	case <-s.lastCommands:
		// VLAN module loading commands is one of the first things the
		// worker does, so if it happened, we can assume commands are
		// executed.
		c.Assert(s.vlanModuleLoaded, jc.IsTrue)
		c.Assert(nw.IsVLANModuleLoaded(), jc.IsTrue)
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("commands expected but not executed")
	}
	c.Assert(nw.IsPrimaryInterfaceOrLoopback("lo"), jc.IsTrue)
	c.Assert(nw.IsPrimaryInterfaceOrLoopback("eth0"), jc.IsTrue)
	s.assertHaveConfig(c, nw, "", "eth0", "eth1", "eth1.42", "eth0.69")
}

func (s *networkerSuite) TestPrimaryOrLoopbackInterfacesAreSkipped(c *gc.C) {
	c.Skip("enable once the networker is enabled again")

	// Reset what's considered up, so we can test eth0 and lo are not
	// touched.
	s.upInterfaces = make(set.Strings)
	s.interfacesWithAddress = make(set.Strings)

	nw, _ := s.newCustomNetworker(c, s.apiFacade, s.stateMachine.Id(), true, false)
	defer worker.Stop(nw)

	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case <-s.lastCommands:
			if !s.vlanModuleLoaded {
				// VLAN module loading commands is one of the first things
				// the worker does, so if hasn't happened, we wait a bit more.
				continue
			}
			c.Assert(s.upInterfaces.Contains("lo"), jc.IsFalse)
			c.Assert(s.upInterfaces.Contains("eth0"), jc.IsFalse)
			if s.upInterfaces.Contains("eth1") {
				// If we run ifup eth1, we successfully skipped lo and
				// eth0.
				s.assertHaveConfig(c, nw, "", "eth0", "eth1", "eth1.42", "eth0.69")
				return
			}
		case <-timeout:
			c.Fatalf("commands expected but not executed")
		}
	}
}

func (s *networkerSuite) TestDisabledInterfacesAreBroughtDown(c *gc.C) {
	c.Skip("enable once the networker is enabled again")

	// Simulate eth1 is up and then disable it, so we can test it's
	// brought down. Also test the VLAN interface eth1.42 is also
	// brought down, as it's physical interface eth1 is disabled.
	s.upInterfaces = set.NewStrings("lo", "eth0", "eth1")
	s.interfacesWithAddress = set.NewStrings("lo", "eth0", "eth1")
	s.machineInterfaces[2].Flags |= net.FlagUp
	ifaces, err := s.stateMachine.NetworkInterfaces()
	c.Assert(err, jc.ErrorIsNil)
	err = ifaces[1].Disable()
	c.Assert(err, jc.ErrorIsNil)
	// We verify that setting the parent physical interface to
	// disabled leads to setting any VLAN intefaces depending on it to
	// get disabled as well.
	err = ifaces[2].Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ifaces[2].IsDisabled(), jc.IsTrue)

	nw, _ := s.newCustomNetworker(c, s.apiFacade, s.stateMachine.Id(), true, false)
	defer worker.Stop(nw)

	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case cmds := <-s.lastCommands:
			if !strings.Contains(strings.Join(cmds, " "), "ifdown") {
				// No down commands yet, keep waiting.
				continue
			}
			c.Assert(s.upInterfaces.Contains("eth1"), jc.IsFalse)
			c.Assert(s.machineInterfaces[2].Flags&net.FlagUp, gc.Equals, net.Flags(0))
			c.Assert(s.upInterfaces.Contains("eth1.42"), jc.IsFalse)
			s.assertNoConfig(c, nw, "eth1", "eth1.42")
			s.assertHaveConfig(c, nw, "", "eth0", "eth0.69")
			return
		case <-timeout:
			c.Fatalf("commands expected but not executed")
		}
	}
}

func (s *networkerSuite) TestIsRunningInLXC(c *gc.C) {
	tests := []struct {
		machineId string
		result    bool
	}{
		{"0", false},
		{"1/lxc/0", true},
		{"2/kvm/1", false},
		{"3/lxc/0/lxc/1", true},
		{"4/lxc/0/kvm/1", false},
		{"5/lxc/1/kvm/1/lxc/3", true},
	}
	for i, t := range tests {
		c.Logf("test %d: %q -> %v", i, t.machineId, t.result)
		c.Check(networker.IsRunningInLXC(t.machineId), gc.Equals, t.result)
	}
}

func (s *networkerSuite) TestNoModprobeWhenRunningInLXC(c *gc.C) {
	c.Skip("enable once the networker is enabled again")

	// Create a new container.
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	lxcMachine, err := s.State.AddMachineInsideMachine(template, s.stateMachine.Id(), instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = lxcMachine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	lxcInterfaces := []state.NetworkInterfaceInfo{{
		MACAddress:    "aa:bb:cc:dd:02:f0",
		InterfaceName: "eth0.123",
		NetworkName:   "vlan123",
		IsVirtual:     true,
		Disabled:      false,
	}}
	s.machineInterfaces = []net.Interface{
		{Index: 1, MTU: 65535, Name: "lo", Flags: net.FlagUp | net.FlagLoopback},
		{Index: 2, MTU: 1500, Name: "eth0", Flags: net.FlagUp},
	}

	err = lxcMachine.SetInstanceInfo("i-am-lxc", "fake_nonce", nil, s.stateNetworks, lxcInterfaces, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Login to the API as the machine agent of lxcMachine.
	lxcState := s.OpenAPIAsMachine(c, lxcMachine.Tag(), password, "fake_nonce")
	c.Assert(lxcState, gc.NotNil)
	lxcFacade := lxcState.Networker()
	c.Assert(lxcFacade, gc.NotNil)

	// Create and setup networker for the LXC machine.
	nw, _ := s.newCustomNetworker(c, lxcFacade, lxcMachine.Id(), true, true)
	defer worker.Stop(nw)

	timeout := time.After(coretesting.LongWait)
	for {
		select {
		case cmds := <-s.lastCommands:
			if !s.upInterfaces.Contains("eth0.123") {
				c.Fatalf("expected command ifup eth0.123, got %v", cmds)
			}
			c.Assert(s.vlanModuleLoaded, jc.IsFalse)
			c.Assert(nw.IsVLANModuleLoaded(), jc.IsFalse)
			s.assertHaveConfig(c, nw, "", "eth0.123")
			s.assertNoConfig(c, nw, "lo", "eth0")
			return
		case <-timeout:
			c.Fatalf("no commands executed!")
		}
	}
}

type mockConfig struct {
	agent.Config
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(machineId string) agent.Config {
	return &mockConfig{tag: names.NewMachineTag(machineId)}
}

// Create several networks.
func (s *networkerSuite) setUpNetworks(c *gc.C) {
	s.stateNetworks = []state.NetworkInfo{{
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

func (s *networkerSuite) setUpMachine(c *gc.C) {
	var err error
	s.stateMachine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.stateMachine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.stateInterfaces = []state.NetworkInterfaceInfo{{
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
	err = s.stateMachine.SetInstanceInfo("i-am", "fake_nonce", nil, s.stateNetworks, s.stateInterfaces, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.apiState = s.OpenAPIAsMachine(c, s.stateMachine.Tag(), password, "fake_nonce")
	c.Assert(s.apiState, gc.NotNil)
}

func (s *networkerSuite) executeCommandsHook(c *gc.C, commands []string) error {
	markUp := func(name string, isUp bool) {
		for i, iface := range s.machineInterfaces {
			if iface.Name == name {
				if isUp {
					iface.Flags |= net.FlagUp
				} else {
					iface.Flags &= ^net.FlagUp
				}
				s.machineInterfaces[i] = iface
				return
			}
		}
	}
	for _, cmd := range commands {
		args := strings.Split(cmd, " ")
		if len(args) >= 2 {
			what, name := args[0], args[1]
			switch what {
			case "ifup":
				s.upInterfaces.Add(name)
				s.interfacesWithAddress.Add(name)
				markUp(name, true)
				c.Logf("bringing %q up", name)
			case "ifdown":
				s.upInterfaces.Remove(name)
				s.interfacesWithAddress.Remove(name)
				markUp(name, false)
				c.Logf("bringing %q down", name)
			}
		}
		if strings.Contains(cmd, "modprobe 8021q") {
			s.vlanModuleLoaded = true
			c.Logf("VLAN module loaded")
		}
	}
	// Send the commands without blocking.
	select {
	case s.lastCommands <- commands:
	default:
	}
	return nil
}

func (s *networkerSuite) newCustomNetworker(
	c *gc.C,
	facade apinetworker.State,
	machineId string,
	intrusiveMode bool,
	initInterfaces bool,
) (*networker.Networker, string) {
	if initInterfaces {
		s.upInterfaces = set.NewStrings("lo", "eth0")
		s.interfacesWithAddress = set.NewStrings("lo", "eth0")
	}
	s.lastCommands = make(chan []string)
	s.vlanModuleLoaded = false
	configDir := c.MkDir()

	nw, err := networker.NewNetworker(facade, agentConfig(machineId), intrusiveMode, configDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nw, gc.NotNil)

	return nw, configDir
}

func (s *networkerSuite) newNetworker(c *gc.C, canWriteConfig bool) *networker.Networker {
	nw, _ := s.newCustomNetworker(c, s.apiFacade, s.stateMachine.Id(), canWriteConfig, true)
	return nw
}

func (s *networkerSuite) assertNoConfig(c *gc.C, nw *networker.Networker, interfaceNames ...string) {
	for _, name := range interfaceNames {
		fullPath := nw.ConfigFile(name)
		_, err := os.Stat(fullPath)
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}
}

func (s *networkerSuite) assertHaveConfig(c *gc.C, nw *networker.Networker, interfaceNames ...string) {
	for _, name := range interfaceNames {
		fullPath := nw.ConfigFile(name)
		_, err := os.Stat(fullPath)
		c.Assert(err, jc.ErrorIsNil)
	}
}
