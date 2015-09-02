// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc/mock"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	"github.com/juju/juju/juju/arch"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
)

type lxcSuite struct {
	lxctesting.TestSuite
	events     chan mock.Event
	eventsDone chan struct{}
}

type lxcBrokerSuite struct {
	lxcSuite
	broker      environs.InstanceBroker
	agentConfig agent.ConfigSetterWriter
	api         *fakeAPI
}

var _ = gc.Suite(&lxcBrokerSuite{})

func (s *lxcSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxc tests on windows")
	}
	s.events = make(chan mock.Event)
	s.eventsDone = make(chan struct{})
	go func() {
		defer close(s.eventsDone)
		for event := range s.events {
			c.Output(3, fmt.Sprintf("lxc event: <%s, %s>", event.Action, event.InstanceId))
		}
	}()
	s.TestSuite.ContainerFactory.AddListener(s.events)
}

func (s *lxcSuite) TearDownTest(c *gc.C) {
	close(s.events)
	<-s.eventsDone
	s.TestSuite.TearDownTest(c)
}

func (s *lxcBrokerSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxc tests on windows")
	}
	s.lxcSuite.SetUpTest(c)
	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           "/not/used/here",
			Tag:               names.NewMachineTag("1"),
			UpgradedToVersion: version.Current.Number,
			Password:          "dummy-secret",
			Nonce:             "nonce",
			APIAddresses:      []string{"10.0.0.1:1234"},
			CACert:            coretesting.CACert,
			Environment:       coretesting.EnvironmentTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	managerConfig := container.ManagerConfig{
		container.ConfigName: "juju",
		"log-dir":            c.MkDir(),
		"use-clone":          "false",
	}
	s.api = NewFakeAPI()
	s.broker, err = provisioner.NewLxcBroker(s.api, s.agentConfig, managerConfig, nil, false, 0)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lxcBrokerSuite) instanceConfig(c *gc.C, machineId string) *instancecfg.InstanceConfig {
	machineNonce := "fake-nonce"
	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, machineNonce, "released", "quantal", true, nil, stateInfo, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the <rootfs>/etc/network path exists.
	containertesting.EnsureLXCRootFSEtcNetwork(c, "juju-"+names.NewMachineTag(machineId).String())
	return instanceConfig
}

func (s *lxcBrokerSuite) startInstance(c *gc.C, machineId string, volumes []storage.VolumeParams) instance.Instance {
	instanceConfig := s.instanceConfig(c, machineId)
	cons := constraints.Value{}
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	result, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:    cons,
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		Volumes:        volumes,
	})
	c.Assert(err, jc.ErrorIsNil)
	return result.Instance
}

func (s *lxcBrokerSuite) maintainInstance(c *gc.C, machineId string, volumes []storage.VolumeParams) {
	instanceConfig := s.instanceConfig(c, machineId)
	cons := constraints.Value{}
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	err := s.broker.MaintainInstance(environs.StartInstanceParams{
		Constraints:    cons,
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
		Volumes:        volumes,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lxcBrokerSuite) assertDefaultStorageConfig(c *gc.C, lxc instance.Instance) {
	config := filepath.Join(s.LxcDir, string(lxc.Id()), "config")
	AssertFileContents(c, gc.Not(jc.Contains), config, "lxc.aa_profile = lxc-container-default-with-mounting")
}

func (s *lxcBrokerSuite) assertDefaultNetworkConfig(c *gc.C, lxc instance.Instance) {
	lxc_conf := filepath.Join(s.ContainerDir, string(lxc.Id()), "lxc.conf")
	expect := []string{
		"lxc.network.type = veth",
		"lxc.network.link = lxcbr0",
	}
	AssertFileContains(c, lxc_conf, expect...)
}

func (s *lxcBrokerSuite) TestStartInstance(c *gc.C) {
	machineId := "1/lxc/0"
	s.SetFeatureFlags(feature.AddressAllocation)
	lxc := s.startInstance(c, machineId, nil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxc-0")},
	}, {
		FuncName: "ContainerConfig",
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	s.assertDefaultNetworkConfig(c, lxc)
	s.assertDefaultStorageConfig(c, lxc)
}

func (s *lxcBrokerSuite) TestStartInstanceAddressAllocationDisabled(c *gc.C) {
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId, nil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	s.assertDefaultNetworkConfig(c, lxc)
	s.assertDefaultStorageConfig(c, lxc)
}

func (s *lxcBrokerSuite) TestMaintainInstance(c *gc.C) {
	machineId := "1/lxc/0"
	s.SetFeatureFlags(feature.AddressAllocation)
	lxc := s.startInstance(c, machineId, nil)
	s.api.ResetCalls()

	s.maintainInstance(c, machineId, nil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "GetContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxc-0")},
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	s.assertDefaultNetworkConfig(c, lxc)
	s.assertDefaultStorageConfig(c, lxc)
}

func (s *lxcBrokerSuite) TestMaintainInstanceAddressAllocationDisabled(c *gc.C) {
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId, nil)
	s.api.ResetCalls()

	s.maintainInstance(c, machineId, nil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	s.assertDefaultNetworkConfig(c, lxc)
	s.assertDefaultStorageConfig(c, lxc)
}

func (s *lxcBrokerSuite) TestStartInstanceWithStorage(c *gc.C) {
	s.api.fakeContainerConfig.AllowLXCLoopMounts = true
	s.SetFeatureFlags(feature.AddressAllocation)

	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId, []storage.VolumeParams{{Provider: provider.LoopProviderType}})
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxc-0")},
	}, {
		FuncName: "ContainerConfig",
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	// Check storage config.
	config := filepath.Join(s.LxcDir, string(lxc.Id()), "config")
	AssertFileContents(c, jc.Contains, config, "lxc.aa_profile = lxc-container-default-with-mounting")
}

func (s *lxcBrokerSuite) TestStartInstanceLoopMountsDisallowed(c *gc.C) {
	s.api.fakeContainerConfig.AllowLXCLoopMounts = false
	s.SetFeatureFlags(feature.AddressAllocation)
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId, []storage.VolumeParams{{Provider: provider.LoopProviderType}})
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-lxc-0")},
	}, {
		FuncName: "ContainerConfig",
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	s.assertDefaultStorageConfig(c, lxc)
}

func (s *lxcBrokerSuite) TestStartInstanceHostArch(c *gc.C) {
	instanceConfig := s.instanceConfig(c, "1/lxc/0")

	// Patch the host's arch, so the LXC broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}, {
		Version: version.MustParseBinary("2.3.4-quantal-ppc64el"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-ppc64el.tgz",
	}}
	_, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceConfig.Tools.Version.Arch, gc.Equals, arch.PPC64EL)
}

func (s *lxcBrokerSuite) TestStartInstanceToolsArchNotFound(c *gc.C) {
	instanceConfig := s.instanceConfig(c, "1/lxc/0")

	// Patch the host's arch, so the LXC broker will filter tools.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	_, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
	})
	c.Assert(err, gc.ErrorMatches, "need tools for arch ppc64el, only found \\[amd64\\]")
}

func (s *lxcBrokerSuite) TestStartInstanceWithBridgeEnviron(c *gc.C) {
	s.agentConfig.SetValue(agent.LxcBridge, "br0")
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId, nil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}})
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	// Uses default network config
	lxc_conf := filepath.Join(s.ContainerDir, string(lxc.Id()), "lxc.conf")
	expect := []string{
		"lxc.network.type = veth",
		"lxc.network.link = br0",
	}
	AssertFileContains(c, lxc_conf, expect...)
}

func (s *lxcBrokerSuite) TestStartInstancePopulatesNetworkInfo(c *gc.C) {
	s.SetFeatureFlags(feature.AddressAllocation)
	s.PatchValue(provisioner.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		return []net.Addr{&fakeAddr{"0.1.2.1/24"}}, nil
	})
	fakeResolvConf := filepath.Join(c.MkDir(), "resolv.conf")
	err := ioutil.WriteFile(fakeResolvConf, []byte("nameserver ns1.dummy\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(provisioner.ResolvConf, fakeResolvConf)

	instanceConfig := s.instanceConfig(c, "42")
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	result, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:    constraints.Value{},
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.NetworkInfo, gc.HasLen, 1)
	iface := result.NetworkInfo[0]
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(iface, jc.DeepEquals, network.InterfaceInfo{
		DeviceIndex:    0,
		CIDR:           "0.1.2.0/24",
		ConfigType:     network.ConfigStatic,
		InterfaceName:  "eth0", // generated from the device index.
		MACAddress:     "aa:bb:cc:dd:ee:ff",
		DNSServers:     network.NewAddresses("ns1.dummy"),
		Address:        network.NewAddress("0.1.2.3"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
		NetworkName:    network.DefaultPrivate,
		ProviderId:     network.DefaultProviderId,
	})
}

func (s *lxcBrokerSuite) TestStopInstance(c *gc.C) {
	lxc0 := s.startInstance(c, "1/lxc/0", nil)
	lxc1 := s.startInstance(c, "1/lxc/1", nil)
	lxc2 := s.startInstance(c, "1/lxc/2", nil)

	s.assertInstances(c, lxc0, lxc1, lxc2)
	err := s.broker.StopInstances(lxc0.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertInstances(c, lxc1, lxc2)
	c.Assert(s.lxcContainerDir(lxc0), jc.DoesNotExist)
	c.Assert(s.lxcRemovedContainerDir(lxc0), jc.IsDirectory)

	err = s.broker.StopInstances(lxc1.Id(), lxc2.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertInstances(c)
}

func (s *lxcBrokerSuite) TestAllInstances(c *gc.C) {
	lxc0 := s.startInstance(c, "1/lxc/0", nil)
	lxc1 := s.startInstance(c, "1/lxc/1", nil)
	s.assertInstances(c, lxc0, lxc1)

	err := s.broker.StopInstances(lxc1.Id())
	c.Assert(err, jc.ErrorIsNil)
	lxc2 := s.startInstance(c, "1/lxc/2", nil)
	s.assertInstances(c, lxc0, lxc2)
}

func (s *lxcBrokerSuite) assertInstances(c *gc.C, inst ...instance.Instance) {
	results, err := s.broker.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	instancetest.MatchInstances(c, results, inst...)
}

func (s *lxcBrokerSuite) lxcContainerDir(inst instance.Instance) string {
	return filepath.Join(s.ContainerDir, string(inst.Id()))
}

func (s *lxcBrokerSuite) lxcRemovedContainerDir(inst instance.Instance) string {
	return filepath.Join(s.RemovedDir, string(inst.Id()))
}

func (s *lxcBrokerSuite) TestLocalDNSServers(c *gc.C) {
	fakeConf := filepath.Join(c.MkDir(), "resolv.conf")
	s.PatchValue(provisioner.ResolvConf, fakeConf)

	// If config is missing, that's OK.
	dnses, dnsSearch, err := provisioner.LocalDNSServers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dnses, gc.HasLen, 0)
	c.Assert(dnsSearch, gc.Equals, "")

	// Enter some data in fakeConf.
	data := `
 anything else is ignored
  # comments are ignored
  nameserver  0.1.2.3  # that's parsed
search  foo.baz # comment ignored
# nameserver 42.42.42.42 - ignored as well
nameserver 8.8.8.8
nameserver example.com # comment after is ok
`
	err = ioutil.WriteFile(fakeConf, []byte(data), 0644)
	c.Assert(err, jc.ErrorIsNil)

	dnses, dnsSearch, err = provisioner.LocalDNSServers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dnses, jc.DeepEquals, network.NewAddresses(
		"0.1.2.3", "8.8.8.8", "example.com",
	))
	c.Assert(dnsSearch, gc.Equals, "foo.baz")
}

func (s *lxcBrokerSuite) TestMustParseTemplate(c *gc.C) {
	f := func() { provisioner.MustParseTemplate("", "{{invalid}") }
	c.Assert(f, gc.PanicMatches, `template: :1: function "invalid" not defined`)

	tmpl := provisioner.MustParseTemplate("name", "X={{.X}}")
	c.Assert(tmpl, gc.NotNil)
	c.Assert(tmpl.Name(), gc.Equals, "name")

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, struct{ X string }{"42"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.String(), gc.Equals, "X=42")
}

func (s *lxcBrokerSuite) TestRunTemplateCommand(c *gc.C) {
	for i, test := range []struct {
		source        string
		exitNonZeroOK bool
		data          interface{}
		exitCode      int
		expectErr     string
	}{{
		source:        "echo {{.Name}}",
		exitNonZeroOK: false,
		data:          struct{ Name string }{"foo"},
		exitCode:      0,
	}, {
		source:        "exit {{.Code}}",
		exitNonZeroOK: false,
		data:          struct{ Code int }{123},
		exitCode:      123,
		expectErr:     `command "exit 123" failed with exit code 123`,
	}, {
		source:        "exit {{.Code}}",
		exitNonZeroOK: true,
		data:          struct{ Code int }{56},
		exitCode:      56,
	}, {
		source:        "exit 42",
		exitNonZeroOK: true,
		exitCode:      42,
	}, {
		source:        "some-invalid-command",
		exitNonZeroOK: false,
		exitCode:      127, // returned by bash.
		expectErr:     `command "some-invalid-command" failed with exit code 127`,
	}} {
		c.Logf("test %d: %q -> %d", i, test.source, test.exitCode)
		t, err := template.New(fmt.Sprintf("test %d", i)).Parse(test.source)
		if !c.Check(err, jc.ErrorIsNil, gc.Commentf("parsing %q", test.source)) {
			continue
		}
		exitCode, err := provisioner.RunTemplateCommand(t, test.exitNonZeroOK, test.data)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(exitCode, gc.Equals, test.exitCode)
	}
}

func (s *lxcBrokerSuite) TestSetupRoutesAndIPTablesInvalidArgs(c *gc.C) {
	// Isolate the test from the host machine.
	gitjujutesting.PatchExecutableThrowError(c, s, "iptables", 42)
	gitjujutesting.PatchExecutableThrowError(c, s, "ip", 123)

	// Check that all the arguments are verified to be non-empty.
	expectStartupErr := "primaryNIC, primaryAddr, bridgeName, and ifaceInfo must be all set"
	emptyIfaceInfo := []network.InterfaceInfo{}
	for i, test := range []struct {
		about       string
		primaryNIC  string
		primaryAddr network.Address
		bridgeName  string
		ifaceInfo   []network.InterfaceInfo
		expectErr   string
	}{{
		about:       "all empty",
		primaryNIC:  "",
		primaryAddr: network.Address{},
		bridgeName:  "",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryNIC empty",
		primaryNIC:  "nic",
		primaryAddr: network.Address{},
		bridgeName:  "",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryAddr empty",
		primaryNIC:  "",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but bridgeName empty",
		primaryNIC:  "",
		primaryAddr: network.Address{},
		bridgeName:  "bridge",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryNIC and bridgeName empty",
		primaryNIC:  "nic",
		primaryAddr: network.Address{},
		bridgeName:  "bridge",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryNIC and primaryAddr empty",
		primaryNIC:  "nic",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryAddr and bridgeName empty",
		primaryNIC:  "",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "bridge",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all set except ifaceInfo",
		primaryNIC:  "nic",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "bridge",
		ifaceInfo:   nil,
		expectErr:   expectStartupErr,
	}, {
		about:       "all empty (ifaceInfo set but empty)",
		primaryNIC:  "",
		primaryAddr: network.Address{},
		bridgeName:  "",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryNIC empty (ifaceInfo set but empty)",
		primaryNIC:  "nic",
		primaryAddr: network.Address{},
		bridgeName:  "",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but primaryAddr empty (ifaceInfo set but empty)",
		primaryNIC:  "",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "all but bridgeName empty (ifaceInfo set but empty)",
		primaryNIC:  "",
		primaryAddr: network.Address{},
		bridgeName:  "bridge",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "just primaryAddr is empty and ifaceInfo set but empty",
		primaryNIC:  "nic",
		primaryAddr: network.Address{},
		bridgeName:  "bridge",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "just bridgeName is empty and ifaceInfo set but empty",
		primaryNIC:  "nic",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "just primaryNIC is empty and ifaceInfo set but empty",
		primaryNIC:  "",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "bridge",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "all set except ifaceInfo, which is set but empty",
		primaryNIC:  "nic",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "bridge",
		ifaceInfo:   emptyIfaceInfo,
		expectErr:   expectStartupErr,
	}, {
		about:       "all set, but ifaceInfo has empty Address",
		primaryNIC:  "nic",
		primaryAddr: network.NewAddress("0.1.2.1"),
		bridgeName:  "bridge",
		// No Address set.
		ifaceInfo: []network.InterfaceInfo{{DeviceIndex: 0}},
		expectErr: `container IP "" must be set`,
	}} {
		c.Logf("test %d: %s", i, test.about)
		err := provisioner.SetupRoutesAndIPTables(
			test.primaryNIC,
			test.primaryAddr,
			test.bridgeName,
			test.ifaceInfo,
			false, // TODO(dimitern): Untested.
		)
		c.Check(err, gc.ErrorMatches, test.expectErr)
	}
}

func (s *lxcBrokerSuite) TestSetupRoutesAndIPTablesIPTablesCheckError(c *gc.C) {
	// Isolate the test from the host machine.
	gitjujutesting.PatchExecutableThrowError(c, s, "iptables", 42)
	gitjujutesting.PatchExecutableThrowError(c, s, "ip", 123)

	ifaceInfo := []network.InterfaceInfo{{
		Address: network.NewAddress("0.1.2.3"),
	}}

	addr := network.NewAddress("0.1.2.1")
	err := provisioner.SetupRoutesAndIPTables("nic", addr, "bridge", ifaceInfo, false)
	c.Assert(err, gc.ErrorMatches, "iptables failed with unexpected exit code 42")
}

func (s *lxcBrokerSuite) TestSetupRoutesAndIPTablesIPTablesAddError(c *gc.C) {
	// Isolate the test from the host machine. Patch iptables with a
	// script which returns code=1 for the check but fails when adding
	// the rule.
	script := `if [[ "$3" == "-C" ]]; then exit 1; else exit 42; fi`
	gitjujutesting.PatchExecutable(c, s, "iptables", script)
	gitjujutesting.PatchExecutableThrowError(c, s, "ip", 123)

	fakeptablesRules := map[string]provisioner.IptablesRule{
		"IPTablesSNAT": {
			"nat",
			"POSTROUTING",
			"{{.HostIF}} {{.HostIP}}",
		},
	}
	s.PatchValue(provisioner.IptablesRules, fakeptablesRules)

	ifaceInfo := []network.InterfaceInfo{{
		Address: network.NewAddress("0.1.2.3"),
	}}

	addr := network.NewAddress("0.1.2.1")
	err := provisioner.SetupRoutesAndIPTables("nic", addr, "bridge", ifaceInfo, false)
	c.Assert(err, gc.ErrorMatches, `command "iptables -t nat -I .*" failed with exit code 42`)
}

func (s *lxcBrokerSuite) TestSetupRoutesAndIPTablesIPRouteError(c *gc.C) {
	// Isolate the test from the host machine.
	// Returning code=0 from iptables means we won't add a rule.
	gitjujutesting.PatchExecutableThrowError(c, s, "iptables", 0)
	gitjujutesting.PatchExecutableThrowError(c, s, "ip", 123)

	ifaceInfo := []network.InterfaceInfo{{
		Address: network.NewAddress("0.1.2.3"),
	}}

	addr := network.NewAddress("0.1.2.1")
	err := provisioner.SetupRoutesAndIPTables("nic", addr, "bridge", ifaceInfo, false)
	c.Assert(err, gc.ErrorMatches,
		`command "ip route add 0.1.2.3 dev bridge" failed with exit code 123`,
	)
}

func (s *lxcBrokerSuite) TestSetupRoutesAndIPTablesAddsRuleIfMissing(c *gc.C) {
	// Isolate the test from the host machine. Because PatchExecutable
	// does not allow us to assert on subsequent executions of the
	// same binary, we need to replace the iptables commands with
	// separate ones. The check returns code=1 to trigger calling
	// add.
	fakeptablesRules := map[string]provisioner.IptablesRule{
		"IPTablesSNAT": {
			"nat",
			"POSTROUTING",
			"{{.HostIF}} {{.HostIP}}",
		},
	}
	s.PatchValue(provisioner.IptablesRules, fakeptablesRules)

	gitjujutesting.PatchExecutableAsEchoArgs(c, s, "iptables", 1, 0)
	gitjujutesting.PatchExecutableAsEchoArgs(c, s, "ip")

	ifaceInfo := []network.InterfaceInfo{{
		Address: network.NewAddress("0.1.2.3"),
	}}

	addr := network.NewAddress("0.1.2.1")
	err := provisioner.SetupRoutesAndIPTables("nic", addr, "bridge", ifaceInfo, false)
	c.Assert(err, jc.ErrorIsNil)

	// Now verify the expected commands - since check returns 1, add
	// will be called before ip route add.

	gitjujutesting.AssertEchoArgs(c, "iptables", "-t", "nat", "-C", "POSTROUTING", "nic", "0.1.2.1")
	gitjujutesting.AssertEchoArgs(c, "iptables", "-t", "nat", "-I", "POSTROUTING", "1", "nic", "0.1.2.1")
	gitjujutesting.AssertEchoArgs(c, "ip", "route", "add", "0.1.2.3", "dev", "bridge")
}

func (s *lxcBrokerSuite) TestDiscoverPrimaryNICNetInterfacesError(c *gc.C) {
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return nil, errors.New("boom!")
	})

	nic, addr, err := provisioner.DiscoverPrimaryNIC()
	c.Assert(err, gc.ErrorMatches, "cannot get network interfaces: boom!")
	c.Assert(nic, gc.Equals, "")
	c.Assert(addr, jc.DeepEquals, network.Address{})
}

func (s *lxcBrokerSuite) TestDiscoverPrimaryNICInterfaceAddrsError(c *gc.C) {
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return []net.Interface{{
			Index: 0,
			Name:  "fake",
			Flags: net.FlagUp,
		}}, nil
	})
	s.PatchValue(provisioner.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		return nil, errors.New("boom!")
	})

	nic, addr, err := provisioner.DiscoverPrimaryNIC()
	c.Assert(err, gc.ErrorMatches, `cannot get "fake" addresses: boom!`)
	c.Assert(nic, gc.Equals, "")
	c.Assert(addr, jc.DeepEquals, network.Address{})
}

func (s *lxcBrokerSuite) TestDiscoverPrimaryNICInvalidAddr(c *gc.C) {
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return []net.Interface{{
			Index: 0,
			Name:  "fake",
			Flags: net.FlagUp,
		}}, nil
	})
	s.PatchValue(provisioner.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		return []net.Addr{&fakeAddr{}}, nil
	})

	nic, addr, err := provisioner.DiscoverPrimaryNIC()
	c.Assert(err, gc.ErrorMatches, `cannot parse address "fakeAddr": invalid CIDR address: fakeAddr`)
	c.Assert(nic, gc.Equals, "")
	c.Assert(addr, jc.DeepEquals, network.Address{})
}

func (s *lxcBrokerSuite) TestDiscoverPrimaryNICInterfaceNotFound(c *gc.C) {
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return nil, nil
	})

	nic, addr, err := provisioner.DiscoverPrimaryNIC()
	c.Assert(err, gc.ErrorMatches, "cannot detect the primary network interface")
	c.Assert(nic, gc.Equals, "")
	c.Assert(addr, jc.DeepEquals, network.Address{})
}

type fakeAddr struct{ value string }

func (f *fakeAddr) Network() string { return "net" }
func (f *fakeAddr) String() string {
	if f.value != "" {
		return f.value
	}
	return "fakeAddr"
}

var _ net.Addr = (*fakeAddr)(nil)

func (s *lxcBrokerSuite) TestDiscoverPrimaryNICSuccess(c *gc.C) {
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return []net.Interface{{
			Index: 0,
			Name:  "lo",
			Flags: net.FlagUp | net.FlagLoopback, // up but loopback - ignored.
		}, {
			Index: 1,
			Name:  "if0",
			Flags: net.FlagPointToPoint, // not up - ignored.
		}, {
			Index: 2,
			Name:  "if1",
			Flags: net.FlagUp, // up but no addresses - ignored.
		}, {
			Index: 3,
			Name:  "if2",
			Flags: net.FlagUp, // up and has addresses - returned.
		}}, nil
	})
	s.PatchValue(provisioner.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		// We should be called only for the last two NICs. The first
		// one (if1) won't have addresses, only the last one (if2).
		c.Assert(i, gc.NotNil)
		c.Assert(i.Name, gc.Matches, "if[12]")
		if i.Name == "if2" {
			return []net.Addr{&fakeAddr{"0.1.2.3/24"}}, nil
		}
		// For if1 we return no addresses.
		return nil, nil
	})

	nic, addr, err := provisioner.DiscoverPrimaryNIC()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nic, gc.Equals, "if2")
	c.Assert(addr, jc.DeepEquals, network.NewAddress("0.1.2.3"))
}

func (s *lxcBrokerSuite) TestConfigureContainerNetwork(c *gc.C) {
	// All the pieces used by this func are separately tested, we just
	// test the integration between them.
	s.PatchValue(provisioner.NetInterfaces, func() ([]net.Interface, error) {
		return []net.Interface{{
			Index: 0,
			Name:  "fake0",
			Flags: net.FlagUp,
		}}, nil
	})
	s.PatchValue(provisioner.InterfaceAddrs, func(i *net.Interface) ([]net.Addr, error) {
		return []net.Addr{&fakeAddr{"0.1.2.1/24"}}, nil
	})
	fakeResolvConf := filepath.Join(c.MkDir(), "resolv.conf")
	err := ioutil.WriteFile(fakeResolvConf, []byte("nameserver ns1.dummy\n"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(provisioner.ResolvConf, fakeResolvConf)

	// When ifaceInfo is not empty it shouldn't do anything and both
	// the error and the result are nil.
	ifaceInfo := []network.InterfaceInfo{{DeviceIndex: 0}}
	// First call as if we are configuring the container for the first time
	result, err := provisioner.ConfigureContainerNetwork("42", "bridge", s.api, ifaceInfo, true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.IsNil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{})

	// Next call as if the container has already been configured.
	s.api.ResetCalls()
	result, err = provisioner.ConfigureContainerNetwork("42", "bridge", s.api, ifaceInfo, false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.IsNil)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{})

	// Call as if the container already has a network configuration, but doesn't.
	s.api.ResetCalls()
	s.api.SetErrors(errors.NotProvisionedf("machine-42 has no network provisioning info"))
	ifaceInfo = []network.InterfaceInfo{}
	result, err = provisioner.ConfigureContainerNetwork("42", "bridge", s.api, ifaceInfo, false, false)
	c.Assert(err, gc.ErrorMatches, "machine-42 has no network provisioning info not provisioned")
	c.Assert(result, jc.DeepEquals, []network.InterfaceInfo{})
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "GetContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("42")},
	}})

	// When it's not empty, result should be populated as expected.
	s.api.ResetCalls()
	result, err = provisioner.ConfigureContainerNetwork("42", "bridge", s.api, ifaceInfo, false, false)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:    0,
		CIDR:           "0.1.2.0/24",
		ConfigType:     network.ConfigStatic,
		InterfaceName:  "eth0", // generated from the device index.
		MACAddress:     "aa:bb:cc:dd:ee:ff",
		DNSServers:     network.NewAddresses("ns1.dummy"),
		Address:        network.NewAddress("0.1.2.3"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
		NetworkName:    network.DefaultPrivate,
		ProviderId:     network.DefaultProviderId,
	}})
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "GetContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("42")},
	}})

	s.api.ResetCalls()
	result, err = provisioner.ConfigureContainerNetwork("42", "bridge", s.api, ifaceInfo, false, false)
	c.Assert(result, gc.HasLen, 1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []network.InterfaceInfo{{
		DeviceIndex:    0,
		CIDR:           "0.1.2.0/24",
		ConfigType:     network.ConfigStatic,
		InterfaceName:  "eth0", // generated from the device index.
		MACAddress:     "aa:bb:cc:dd:ee:ff",
		DNSServers:     network.NewAddresses("ns1.dummy"),
		Address:        network.NewAddress("0.1.2.3"),
		GatewayAddress: network.NewAddress("0.1.2.1"),
		NetworkName:    network.DefaultPrivate,
		ProviderId:     network.DefaultProviderId,
	}})

	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "GetContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("42")},
	}})
}

type lxcProvisionerSuite struct {
	CommonProvisionerSuite
	lxcSuite
	events chan mock.Event
}

var _ = gc.Suite(&lxcProvisionerSuite{})

func (s *lxcProvisionerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping lxc tests on windows")
	}
	s.CommonProvisionerSuite.SetUpSuite(c)
	s.lxcSuite.SetUpSuite(c)
}

func (s *lxcProvisionerSuite) TearDownSuite(c *gc.C) {
	s.lxcSuite.TearDownSuite(c)
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func (s *lxcProvisionerSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.lxcSuite.SetUpTest(c)

	s.events = make(chan mock.Event, 25)
	s.ContainerFactory.AddListener(s.events)
}

func (s *lxcProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	// This check in particular leads to tests just hanging
	// indefinitely quite often on i386.
	coretesting.SkipIfI386(c, "lp:1425569")

	var event mock.Event
	s.State.StartSync()
	select {
	case event = <-s.events:
		c.Assert(event.Action, gc.Equals, mock.Created)
		argsSet := set.NewStrings(event.TemplateArgs...)
		c.Assert(argsSet.Contains("imageURL"), jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting the mock container to get created")
	}

	select {
	case event = <-s.events:
		c.Assert(event.Action, gc.Equals, mock.Started)
		err := machine.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting the mock container to start")
	}

	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *lxcProvisionerSuite) expectStopped(c *gc.C, instId string) {
	// This check in particular leads to tests just hanging
	// indefinitely quite often on i386.
	coretesting.SkipIfI386(c, "lp:1425569")

	s.State.StartSync()
	select {
	case event := <-s.events:
		c.Assert(event.Action, gc.Equals, mock.Stopped)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting the mock container to stop")
	}

	select {
	case event := <-s.events:
		c.Assert(event.Action, gc.Equals, mock.Destroyed)
		c.Assert(event.InstanceId, gc.Equals, instId)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout while waiting the mock container to get destroyed")
	}
}

func (s *lxcProvisionerSuite) expectNoEvents(c *gc.C) {
	select {
	case event := <-s.events:
		c.Fatalf("unexpected event %#v", event)
	case <-time.After(coretesting.ShortWait):
		return
	}
}

func (s *lxcProvisionerSuite) TearDownTest(c *gc.C) {
	close(s.events)
	s.lxcSuite.TearDownTest(c)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *lxcProvisionerSuite) newLxcProvisioner(c *gc.C) provisioner.Provisioner {
	parentMachineTag := names.NewMachineTag("0")
	agentConfig := s.AgentConfigForTag(c, parentMachineTag)
	managerConfig := container.ManagerConfig{
		container.ConfigName: "juju",
		"log-dir":            c.MkDir(),
		"use-clone":          "false",
	}
	broker, err := provisioner.NewLxcBroker(s.provisioner, agentConfig, managerConfig, &containertesting.MockURLGetter{}, false, 0)
	c.Assert(err, jc.ErrorIsNil)
	toolsFinder := (*provisioner.GetToolsFinder)(s.provisioner)
	return provisioner.NewContainerProvisioner(instance.LXC, s.provisioner, agentConfig, broker, toolsFinder)
}

func (s *lxcProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newLxcProvisioner(c)
	c.Assert(p.Stop(), gc.IsNil)
}

func (s *lxcProvisionerSuite) TestDoesNotStartEnvironMachines(c *gc.C) {
	p := s.newLxcProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.expectNoEvents(c)
}

func (s *lxcProvisionerSuite) TestDoesNotHaveRetryWatcher(c *gc.C) {
	p := s.newLxcProvisioner(c)
	defer stop(c, p)

	w, err := provisioner.GetRetryWatcher(p)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *lxcProvisionerSuite) addContainer(c *gc.C) *state.Machine {
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, "0", instance.LXC)
	c.Assert(err, jc.ErrorIsNil)
	return container
}

func (s *lxcProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	coretesting.SkipIfI386(c, "lp:1425569")

	p := s.newLxcProvisioner(c)
	defer stop(c, p)

	container := s.addContainer(c)
	name := "juju-" + container.Tag().String()
	containertesting.EnsureLXCRootFSEtcNetwork(c, name)
	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitRemoved(c, container)
}

func (s *lxcProvisionerSuite) TestLXCProvisionerObservesConfigChanges(c *gc.C) {
	p := s.newLxcProvisioner(c)
	defer stop(c, p)
	s.assertProvisionerObservesConfigChanges(c, p)
}

type fakeAPI struct {
	*gitjujutesting.Stub

	fakeContainerConfig params.ContainerConfig
	fakeInterfaceInfo   network.InterfaceInfo
}

var _ provisioner.APICalls = (*fakeAPI)(nil)

var fakeInterfaceInfo network.InterfaceInfo = network.InterfaceInfo{
	DeviceIndex:    0,
	MACAddress:     "aa:bb:cc:dd:ee:ff",
	CIDR:           "0.1.2.0/24",
	InterfaceName:  "dummy0",
	Address:        network.NewAddress("0.1.2.3"),
	GatewayAddress: network.NewAddress("0.1.2.1"),
}

var fakeContainerConfig = params.ContainerConfig{
	UpdateBehavior:          &params.UpdateBehavior{true, true},
	ProviderType:            "fake",
	AuthorizedKeys:          coretesting.FakeAuthKeys,
	SSLHostnameVerification: true,
}

func NewFakeAPI() *fakeAPI {
	return &fakeAPI{
		Stub:                &gitjujutesting.Stub{},
		fakeContainerConfig: fakeContainerConfig,
		fakeInterfaceInfo:   fakeInterfaceInfo,
	}
}

func (f *fakeAPI) ContainerConfig() (params.ContainerConfig, error) {
	f.MethodCall(f, "ContainerConfig")
	if err := f.NextErr(); err != nil {
		return params.ContainerConfig{}, err
	}
	return f.fakeContainerConfig, nil
}

func (f *fakeAPI) PrepareContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "PrepareContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}

func (f *fakeAPI) GetContainerInterfaceInfo(tag names.MachineTag) ([]network.InterfaceInfo, error) {
	f.MethodCall(f, "GetContainerInterfaceInfo", tag)
	if err := f.NextErr(); err != nil {
		return nil, err
	}
	return []network.InterfaceInfo{f.fakeInterfaceInfo}, nil
}
