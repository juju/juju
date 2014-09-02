// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc/mock"
	lxctesting "github.com/juju/juju/container/lxc/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	instancetest "github.com/juju/juju/instance/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
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
}

var _ = gc.Suite(&lxcBrokerSuite{})

func (s *lxcSuite) SetUpTest(c *gc.C) {
	s.TestSuite.SetUpTest(c)
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
		})
	c.Assert(err, gc.IsNil)
	managerConfig := container.ManagerConfig{container.ConfigName: "juju", "use-clone": "false"}
	s.broker, err = provisioner.NewLxcBroker(&fakeAPI{}, s.agentConfig, managerConfig)
	c.Assert(err, gc.IsNil)
}

func (s *lxcBrokerSuite) startInstance(c *gc.C, machineId string) instance.Instance {
	machineNonce := "fake-nonce"
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)
	machineConfig, err := environs.NewMachineConfig(machineId, machineNonce, "released", "quantal", nil, stateInfo, apiInfo)
	c.Assert(err, gc.IsNil)
	cons := constraints.Value{}
	possibleTools := coretools.List{&coretools.Tools{
		Version: version.MustParseBinary("2.3.4-quantal-amd64"),
		URL:     "http://tools.testing.invalid/2.3.4-quantal-amd64.tgz",
	}}
	lxc, _, _, err := s.broker.StartInstance(environs.StartInstanceParams{
		Constraints:   cons,
		Tools:         possibleTools,
		MachineConfig: machineConfig,
	})
	c.Assert(err, gc.IsNil)
	return lxc
}

func (s *lxcBrokerSuite) TestStartInstance(c *gc.C) {
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId)
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	// Uses default network config
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, string(lxc.Id()), "lxc.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.type = veth")
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = lxcbr0")
}

func (s *lxcBrokerSuite) TestStartInstanceWithBridgeEnviron(c *gc.C) {
	s.agentConfig.SetValue(agent.LxcBridge, "br0")
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId)
	c.Assert(lxc.Id(), gc.Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), jc.IsDirectory)
	s.assertInstances(c, lxc)
	// Uses default network config
	lxcConfContents, err := ioutil.ReadFile(filepath.Join(s.ContainerDir, string(lxc.Id()), "lxc.conf"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.type = veth")
	c.Assert(string(lxcConfContents), jc.Contains, "lxc.network.link = br0")
}

func (s *lxcBrokerSuite) TestStopInstance(c *gc.C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	lxc2 := s.startInstance(c, "1/lxc/2")

	err := s.broker.StopInstances(lxc0.Id())
	c.Assert(err, gc.IsNil)
	s.assertInstances(c, lxc1, lxc2)
	c.Assert(s.lxcContainerDir(lxc0), jc.DoesNotExist)
	c.Assert(s.lxcRemovedContainerDir(lxc0), jc.IsDirectory)

	err = s.broker.StopInstances(lxc1.Id(), lxc2.Id())
	c.Assert(err, gc.IsNil)
	s.assertInstances(c)
}

func (s *lxcBrokerSuite) TestAllInstances(c *gc.C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	s.assertInstances(c, lxc0, lxc1)

	err := s.broker.StopInstances(lxc1.Id())
	c.Assert(err, gc.IsNil)
	lxc2 := s.startInstance(c, "1/lxc/2")
	s.assertInstances(c, lxc0, lxc2)
}

func (s *lxcBrokerSuite) assertInstances(c *gc.C, inst ...instance.Instance) {
	results, err := s.broker.AllInstances()
	c.Assert(err, gc.IsNil)
	instancetest.MatchInstances(c, results, inst...)
}

func (s *lxcBrokerSuite) lxcContainerDir(inst instance.Instance) string {
	return filepath.Join(s.ContainerDir, string(inst.Id()))
}

func (s *lxcBrokerSuite) lxcRemovedContainerDir(inst instance.Instance) string {
	return filepath.Join(s.RemovedDir, string(inst.Id()))
}

type lxcProvisionerSuite struct {
	CommonProvisionerSuite
	lxcSuite
	events chan mock.Event
}

var _ = gc.Suite(&lxcProvisionerSuite{})

func (s *lxcProvisionerSuite) SetUpSuite(c *gc.C) {
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

	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	s.events = make(chan mock.Event, 25)
	s.ContainerFactory.AddListener(s.events)
}

func (s *lxcProvisionerSuite) expectStarted(c *gc.C, machine *state.Machine) string {
	s.State.StartSync()
	event := <-s.events
	c.Assert(event.Action, gc.Equals, mock.Created)
	event = <-s.events
	c.Assert(event.Action, gc.Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, gc.IsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *lxcProvisionerSuite) expectStopped(c *gc.C, instId string) {
	s.State.StartSync()
	event := <-s.events
	c.Assert(event.Action, gc.Equals, mock.Stopped)
	event = <-s.events
	c.Assert(event.Action, gc.Equals, mock.Destroyed)
	c.Assert(event.InstanceId, gc.Equals, instId)
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
	managerConfig := container.ManagerConfig{container.ConfigName: "juju", "use-clone": "false"}
	broker, err := provisioner.NewLxcBroker(s.provisioner, agentConfig, managerConfig)
	c.Assert(err, gc.IsNil)
	return provisioner.NewContainerProvisioner(instance.LXC, s.provisioner, agentConfig, broker)
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
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)
	return container
}

func (s *lxcProvisionerSuite) TestContainerStartedAndStopped(c *gc.C) {
	p := s.newLxcProvisioner(c)
	defer stop(c, p)

	container := s.addContainer(c)
	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), gc.IsNil)
	s.expectStopped(c, instId)
	s.waitRemoved(c, container)
}

type fakeAPI struct{}

func (*fakeAPI) ContainerConfig() (params.ContainerConfig, error) {
	return params.ContainerConfig{
		UpdateBehavior:          &params.UpdateBehavior{true, true},
		ProviderType:            "fake",
		AuthorizedKeys:          coretesting.FakeAuthKeys,
		SSLHostnameVerification: true}, nil
}
