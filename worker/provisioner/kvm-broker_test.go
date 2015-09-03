// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/kvm/mock"
	kvmtesting "github.com/juju/juju/container/kvm/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	"github.com/juju/juju/juju/arch"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/provisioner"
)

type kvmSuite struct {
	kvmtesting.TestSuite
	events     chan mock.Event
	eventsDone chan struct{}
}

type kvmBrokerSuite struct {
	kvmSuite
	broker      environs.InstanceBroker
	agentConfig agent.Config
	api         *fakeAPI
}

var _ = gc.Suite(&kvmBrokerSuite{})

func (s *kvmSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.TestSuite.SetUpTest(c)
	s.events = make(chan mock.Event)
	s.eventsDone = make(chan struct{})
	go func() {
		defer close(s.eventsDone)
		for event := range s.events {
			c.Output(3, fmt.Sprintf("kvm event: <%s, %s>", event.Action, event.InstanceId))
		}
	}()
	s.TestSuite.ContainerFactory.AddListener(s.events)
}

func (s *kvmSuite) TearDownTest(c *gc.C) {
	close(s.events)
	<-s.eventsDone
	s.TestSuite.TearDownTest(c)
}

func (s *kvmBrokerSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.kvmSuite.SetUpTest(c)
	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           "/not/used/here",
			Tag:               names.NewUnitTag("ubuntu/1"),
			UpgradedToVersion: version.Current.Number,
			Password:          "dummy-secret",
			Nonce:             "nonce",
			APIAddresses:      []string{"10.0.0.1:1234"},
			CACert:            coretesting.CACert,
			Environment:       coretesting.EnvironmentTag,
		})
	c.Assert(err, jc.ErrorIsNil)
	s.api = NewFakeAPI()
	managerConfig := container.ManagerConfig{container.ConfigName: "juju"}
	s.broker, err = provisioner.NewKvmBroker(s.api, s.agentConfig, managerConfig, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *kvmBrokerSuite) instanceConfig(c *gc.C, machineId string) *instancecfg.InstanceConfig {
	machineNonce := "fake-nonce"
	// To isolate the tests from the host's architecture, we override it here.
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, machineNonce, "released", "quantal", true, nil, stateInfo, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	return instanceConfig
}

func (s *kvmBrokerSuite) startInstance(c *gc.C, machineId string) instance.Instance {
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
	})
	c.Assert(err, jc.ErrorIsNil)
	return result.Instance
}

func (s *kvmBrokerSuite) maintainInstance(c *gc.C, machineId string) {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	instanceConfig, err := instancecfg.NewInstanceConfig(machineId, machineNonce, "released", "quantal", true, nil, stateInfo, apiInfo)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.Value{}
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	err = s.broker.MaintainInstance(environs.StartInstanceParams{
		Constraints:    cons,
		Tools:          possibleTools,
		InstanceConfig: instanceConfig,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *kvmBrokerSuite) TestStartInstance(c *gc.C) {
	machineId := "1/kvm/0"
	s.SetFeatureFlags(feature.AddressAllocation)
	kvm := s.startInstance(c, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "PrepareContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-kvm-0")},
	}, {
		FuncName: "ContainerConfig",
	}})
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	s.assertInstances(c, kvm)
}

func (s *kvmBrokerSuite) TestStartInstanceAddressAllocationDisabled(c *gc.C) {
	machineId := "1/kvm/0"
	kvm := s.startInstance(c, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "ContainerConfig",
	}})
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	s.assertInstances(c, kvm)
}

func (s *kvmBrokerSuite) TestMaintainInstance(c *gc.C) {
	machineId := "1/kvm/0"
	s.SetFeatureFlags(feature.AddressAllocation)
	kvm := s.startInstance(c, machineId)
	s.api.ResetCalls()

	s.maintainInstance(c, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "GetContainerInterfaceInfo",
		Args:     []interface{}{names.NewMachineTag("1-kvm-0")},
	}})
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	s.assertInstances(c, kvm)
}

func (s *kvmBrokerSuite) TestMaintainInstanceAddressAllocationDisabled(c *gc.C) {
	machineId := "1/kvm/0"
	kvm := s.startInstance(c, machineId)
	s.api.ResetCalls()

	s.maintainInstance(c, machineId)
	s.api.CheckCalls(c, []gitjujutesting.StubCall{})
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	s.assertInstances(c, kvm)
}

func (s *kvmBrokerSuite) TestStopInstance(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	kvm2 := s.startInstance(c, "1/kvm/2")

	err := s.broker.StopInstances(kvm0.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertInstances(c, kvm1, kvm2)
	c.Assert(s.kvmContainerDir(kvm0), jc.DoesNotExist)
	c.Assert(s.kvmRemovedContainerDir(kvm0), jc.IsDirectory)

	err = s.broker.StopInstances(kvm1.Id(), kvm2.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.assertInstances(c)
}

func (s *kvmBrokerSuite) TestAllInstances(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	s.assertInstances(c, kvm0, kvm1)

	err := s.broker.StopInstances(kvm1.Id())
	c.Assert(err, jc.ErrorIsNil)
	kvm2 := s.startInstance(c, "1/kvm/2")
	s.assertInstances(c, kvm0, kvm2)
}

func (s *kvmBrokerSuite) assertInstances(c *gc.C, inst ...instance.Instance) {
	results, err := s.broker.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	instancetest.MatchInstances(c, results, inst...)
}

func (s *kvmBrokerSuite) kvmContainerDir(inst instance.Instance) string {
	return filepath.Join(s.ContainerDir, string(inst.Id()))
}

func (s *kvmBrokerSuite) kvmRemovedContainerDir(inst instance.Instance) string {
	return filepath.Join(s.RemovedDir, string(inst.Id()))
}

func (s *kvmBrokerSuite) TestStartInstancePopulatesNetworkInfo(c *gc.C) {
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

type kvmProvisionerSuite struct {
	CommonProvisionerSuite
	kvmSuite
	events chan mock.Event
}

var _ = gc.Suite(&kvmProvisionerSuite{})

func (s *kvmProvisionerSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Skipping kvm tests on windows")
	}
	s.CommonProvisionerSuite.SetUpSuite(c)
	s.kvmSuite.SetUpSuite(c)
}

func (s *kvmProvisionerSuite) TearDownSuite(c *gc.C) {
	s.kvmSuite.TearDownSuite(c)
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func (s *kvmProvisionerSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.kvmSuite.SetUpTest(c)

	s.events = make(chan mock.Event, 25)
	s.ContainerFactory.AddListener(s.events)
}

func (s *kvmProvisionerSuite) nextEvent(c *gc.C) mock.Event {
	select {
	case event := <-s.events:
		return event
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no event arrived")
	}
	panic("not reachable")
}

func (s *kvmProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	// This check in particular leads to tests just hanging
	// indefinitely quite often on i386.
	coretesting.SkipIfI386(c, "lp:1425569")

	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *kvmProvisionerSuite) expectStopped(c *gc.C, instId string) {
	// This check in particular leads to tests just hanging
	// indefinitely quite often on i386.
	coretesting.SkipIfI386(c, "lp:1425569")

	s.State.StartSync()
	event := s.nextEvent(c)
	c.Assert(event.Action, gc.Equals, mock.Stopped)
	c.Assert(event.InstanceId, gc.Equals, instId)
}

func (s *kvmProvisionerSuite) expectNoEvents(c *gc.C) {
	select {
	case event := <-s.events:
		c.Fatalf("unexpected event %#v", event)
	case <-time.After(coretesting.ShortWait):
		return
	}
}

func (s *kvmProvisionerSuite) TearDownTest(c *gc.C) {
	close(s.events)
	s.kvmSuite.TearDownTest(c)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C) provisioner.Provisioner {
	machineTag := names.NewMachineTag("0")
	agentConfig := s.AgentConfigForTag(c, machineTag)
	managerConfig := container.ManagerConfig{container.ConfigName: "juju"}
	broker, err := provisioner.NewKvmBroker(s.provisioner, agentConfig, managerConfig, false)
	c.Assert(err, jc.ErrorIsNil)
	toolsFinder := (*provisioner.GetToolsFinder)(s.provisioner)
	return provisioner.NewContainerProvisioner(instance.KVM, s.provisioner, agentConfig, broker, toolsFinder)
}

func (s *kvmProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newKvmProvisioner(c)
	c.Assert(p.Stop(), gc.IsNil)
}

func (s *kvmProvisionerSuite) TestDoesNotStartEnvironMachines(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.expectNoEvents(c)
}

func (s *kvmProvisionerSuite) TestDoesNotHaveRetryWatcher(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer stop(c, p)

	w, err := provisioner.GetRetryWatcher(p)
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *kvmProvisionerSuite) addContainer(c *gc.C) *state.Machine {
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, "0", instance.KVM)
	c.Assert(err, jc.ErrorIsNil)
	return container
}

func (s *kvmProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	coretesting.SkipIfI386(c, "lp:1425569")

	p := s.newKvmProvisioner(c)
	defer stop(c, p)

	container := s.addContainer(c)

	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitRemoved(c, container)
}

func (s *kvmProvisionerSuite) TestKVMProvisionerObservesConfigChanges(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer stop(c, p)
	s.assertProvisionerObservesConfigChanges(c, p)
}
