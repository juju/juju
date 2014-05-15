// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/kvm/mock"
	kvmtesting "launchpad.net/juju-core/container/kvm/testing"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	instancetest "launchpad.net/juju-core/instance/testing"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
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
	var err error
	s.agentConfig, err = agent.NewAgentConfig(
		agent.AgentConfigParams{
			DataDir:           "/not/used/here",
			Tag:               "tag",
			UpgradedToVersion: version.Current.Number,
			Password:          "dummy-secret",
			Nonce:             "nonce",
			APIAddresses:      []string{"10.0.0.1:1234"},
			CACert:            coretesting.CACert,
		})
	c.Assert(err, gc.IsNil)
	managerConfig := container.ManagerConfig{container.ConfigName: "juju"}
	s.broker, err = provisioner.NewKvmBroker(&fakeAPI{}, tools, s.agentConfig, managerConfig)
	c.Assert(err, gc.IsNil)
}

func (s *kvmBrokerSuite) startInstance(c *gc.C, machineId string) instance.Instance {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig := environs.NewMachineConfig(machineId, machineNonce, nil, nil, stateInfo, apiInfo)
	cons := constraints.Value{}
	possibleTools := s.broker.(coretools.HasTools).Tools("precise")
	kvm, _, _, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:   cons,
		Tools:         possibleTools,
		MachineConfig: machineConfig,
	})
	c.Assert(err, gc.IsNil)
	return kvm
}

func (s *kvmBrokerSuite) TestStopInstance(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	kvm2 := s.startInstance(c, "1/kvm/2")

	err := s.broker.StopInstances(kvm0.Id())
	c.Assert(err, gc.IsNil)
	s.assertInstances(c, kvm1, kvm2)
	c.Assert(s.kvmContainerDir(kvm0), jc.DoesNotExist)
	c.Assert(s.kvmRemovedContainerDir(kvm0), jc.IsDirectory)

	err = s.broker.StopInstances(kvm1.Id(), kvm2.Id())
	c.Assert(err, gc.IsNil)
	s.assertInstances(c)
}

func (s *kvmBrokerSuite) TestAllInstances(c *gc.C) {
	kvm0 := s.startInstance(c, "1/kvm/0")
	kvm1 := s.startInstance(c, "1/kvm/1")
	s.assertInstances(c, kvm0, kvm1)

	err := s.broker.StopInstances(kvm1.Id())
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

	// The kvm provisioner actually needs the machine it is being created on
	// to be in state, in order to get the watcher.
	m, err := s.State.AddMachine(coretesting.FakeDefaultSeries, state.JobHostUnits, state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = m.SetAddresses(instance.NewAddress("0.1.2.3", instance.NetworkUnknown))
	c.Assert(err, gc.IsNil)

	hostPorts := [][]instance.HostPort{{{
		Address: instance.NewAddress("0.1.2.3", instance.NetworkUnknown),
		Port:    1234,
	}}}
	err = s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	s.machineId = m.Id()
	s.APILogin(c, m)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

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

func (s *kvmProvisionerSuite) newKvmProvisioner(c *gc.C) provisioner.Provisioner {
	machineTag := names.MachineTag(s.machineId)
	agentConfig := s.AgentConfigForTag(c, machineTag)
	tools, err := s.provisioner.Tools(agentConfig.Tag())
	c.Assert(err, gc.IsNil)
	managerConfig := container.ManagerConfig{container.ConfigName: "juju"}
	broker, err := provisioner.NewKvmBroker(s.provisioner, tools, agentConfig, managerConfig)
	c.Assert(err, gc.IsNil)
	return provisioner.NewContainerProvisioner(instance.KVM, s.provisioner, agentConfig, broker)
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
	c.Assert(err, gc.IsNil)

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
	container, err := s.State.AddMachineInsideMachine(template, s.machineId, instance.KVM)
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
