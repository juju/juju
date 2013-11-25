// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/kvm/mock"
	kvmtesting "launchpad.net/juju-core/container/kvm/testing"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	instancetest "launchpad.net/juju-core/instance/testing"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/provisioner"
)

type kvmSuite struct {
	kvmtesting.TestSuite
	events chan mock.Event
}

type kvmBrokerSuite struct {
	kvmSuite
	broker      environs.InstanceBroker
	agentConfig agent.Config
}

var _ = gc.Suite(&kvmBrokerSuite{})

func (s *kvmSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
	s.events = make(chan mock.Event)
	go func() {
		for event := range s.events {
			c.Output(3, fmt.Sprintf("kvm event: <%s, %s>", event.Action, event.InstanceId))
		}
	}()
	s.TestSuite.Factory.AddListener(s.events)
}

func (s *kvmSuite) TearDownTest(c *gc.C) {
	close(s.events)
	s.TestSuite.TearDownTest(c)
}

func (s *kvmBrokerSuite) SetUpTest(c *gc.C) {
	s.kvmSuite.SetUpTest(c)
	tools := &coretools.Tools{
		Version: version.MustParseBinary("2.3.4-foo-bar"),
		URL:     "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}
	config := coretesting.EnvironConfig(c)
	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:      "/not/used/here",
			Tag:          "tag",
			Password:     "dummy-secret",
			Nonce:        "nonce",
			APIAddresses: []string{"10.0.0.1:1234"},
			CACert:       []byte(coretesting.CACert),
		})
	c.Assert(err, gc.IsNil)
	s.broker = provisioner.NewKvmBroker(config, tools, s.agentConfig)
}

func (s *kvmBrokerSuite) startInstance(c *gc.C, machineId string) instance.Instance {
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	series := "series"
	nonce := "fake-nonce"
	cons := constraints.Value{}
	kvm, _, err := provider.StartInstance(s.broker, machineId, nonce, series, cons, stateInfo, apiInfo)
	c.Assert(err, gc.IsNil)
	return kvm
}

func (s *kvmBrokerSuite) TestStartInstance(c *gc.C) {
	machineId := "1/kvm/0"
	kvm := s.startInstance(c, machineId)
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	c.Assert(s.kvmContainerDir(kvm), jc.IsDirectory)
	s.assertInstances(c, kvm)
	// Uses default network config
	kvmConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, string(kvm.Id()), "kvm.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(kvmConfContents), jc.Contains, "kvm.network.type = veth")
	c.Assert(string(kvmConfContents), jc.Contains, "kvm.network.link = kvmbr0")
}

func (s *kvmBrokerSuite) TestStartInstanceWithBridgeEnviron(c *gc.C) {
	s.agentConfig.SetValue(agent.KvmBridge, "br0")
	machineId := "1/kvm/0"
	kvm := s.startInstance(c, machineId)
	c.Assert(kvm.Id(), gc.Equals, instance.Id("juju-machine-1-kvm-0"))
	c.Assert(s.kvmContainerDir(kvm), jc.IsDirectory)
	s.assertInstances(c, kvm)
	// Uses default network config
	kvmConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, string(kvm.Id()), "kvm.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(kvmConfContents), jc.Contains, "kvm.network.type = veth")
	c.Assert(string(kvmConfContents), jc.Contains, "kvm.network.link = br0")
}

func (s *kvmBrokerSuite) TestStopInstance(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	kvm2 := s.startInstance(c, "1/kvm/2")

	err := s.broker.StopInstances([]instance.Instance{kvm0})
	c.Assert(err, gc.IsNil)
	s.assertInstances(c, kvm1, kvm2)
	c.Assert(s.kvmContainerDir(kvm0), jc.DoesNotExist)
	c.Assert(s.kvmRemovedContainerDir(kvm0), jc.IsDirectory)

	err = s.broker.StopInstances([]instance.Instance{kvm1, kvm2})
	c.Assert(err, gc.IsNil)
	s.assertInstances(c)
}

func (s *kvmBrokerSuite) TestAllInstances(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	s.assertInstances(c, kvm0, kvm1)

	err := s.broker.StopInstances([]instance.Instance{kvm1})
	c.Assert(err, gc.IsNil)
	kvm2 := s.startInstance(c, "1/kvm/2")
	s.assertInstances(c, kvm0, kvm2)
}

func (s *kvmBrokerSuite) assertInstances(c *gc.C, inst ...instance.Instance) {
	results, err := s.broker.AllInstances()
	c.Assert(err, gc.IsNil)
	instancetest.MatchInstances(c, results, inst...)
}

func (s *kvmBrokerSuite) kvmContainerDir(inst instance.Instance) string {
	return filepath.Join(s.ContainerDir, string(inst.Id()))
}

func (s *kvmBrokerSuite) kvmRemovedContainerDir(inst instance.Instance) string {
	return filepath.Join(s.RemovedDir, string(inst.Id()))
}

type kvmProvisionerSuite struct {
	CommonProvisionerSuite
	kvmSuite
	machineId string
	events    chan mock.Event
}

var _ = gc.Suite(&kvmProvisionerSuite{})

func (s *kvmProvisionerSuite) SetUpSuite(c *gc.C) {
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
	// Write the tools file.
	toolsDir := agenttools.SharedToolsDir(s.DataDir(), version.Current)
	c.Assert(os.MkdirAll(toolsDir, 0755), gc.IsNil)
	urlPath := filepath.Join(toolsDir, "downloaded-url.txt")
	err := ioutil.WriteFile(urlPath, []byte("http://testing.invalid/tools"), 0644)
	c.Assert(err, gc.IsNil)

	// The kvm provisioner actually needs the machine it is being created on
	// to be in state, in order to get the watcher.
	m, err := s.State.AddMachine(config.DefaultSeries, state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	s.machineId = m.Id()

	s.events = make(chan mock.Event, 25)
	s.Factory.AddListener(s.events)
}

func (s *kvmProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	s.State.StartSync()
	event := <-s.events
	c.Assert(event.Action, gc.Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, gc.IsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *kvmProvisionerSuite) expectStopped(c *gc.C, instId string) {
	s.State.StartSync()
	event := <-s.events
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

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C) *provisioner.Provisioner {
	machineTag := names.MachineTag(s.machineId)
	agentConfig := s.AgentConfigForTag(c, machineTag)
	return provisioner.NewProvisioner(provisioner.KVM, s.State, s.machineId, agentConfig)
}

func (s *kvmProvisionerSuite) TestProvisionerStartStop(c *gc.C) {
	p := s.newKvmProvisioner(c)
	c.Assert(p.Stop(), gc.IsNil)
}

func (s *kvmProvisionerSuite) TestDoesNotStartEnvironMachines(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(config.DefaultSeries, state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	s.expectNoEvents(c)
}

func (s *kvmProvisionerSuite) addContainer(c *gc.C) *state.Machine {
	params := state.AddMachineParams{
		ParentId:      s.machineId,
		ContainerType: instance.KVM,
		Series:        config.DefaultSeries,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	return container
}

func (s *kvmProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	p := s.newKvmProvisioner(c)
	defer stop(c, p)

	container := s.addContainer(c)

	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitRemoved(c, container)
}
