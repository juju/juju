// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/provisioner"
)

type lxcSuite struct {
	testing.LoggingSuite
	lxc.TestSuite
	events chan mock.Event
}

type lxcBrokerSuite struct {
	lxcSuite
	broker provisioner.Broker
}

var _ = Suite(&lxcBrokerSuite{})

func (s *lxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.TestSuite.SetUpSuite(c)
}

func (s *lxcSuite) TearDownSuite(c *C) {
	s.TestSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *lxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.TestSuite.SetUpTest(c)
	s.events = make(chan mock.Event)
	go func() {
		for event := range s.events {
			c.Output(3, fmt.Sprintf("lxc event: <%s, %s>", event.Action, event.InstanceId))
		}
	}()
	s.TestSuite.Factory.AddListener(s.events)
}

func (s *lxcSuite) TearDownTest(c *C) {
	close(s.events)
	s.TestSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *lxcBrokerSuite) SetUpTest(c *C) {
	s.lxcSuite.SetUpTest(c)
	tools := &state.Tools{
		Binary: version.MustParseBinary("2.3.4-foo-bar"),
		URL:    "http://tools.testing.invalid/2.3.4-foo-bar.tgz",
	}
	s.broker = provisioner.NewLxcBroker(testing.EnvironConfig(c), tools)
}

func (s *lxcBrokerSuite) startInstance(c *C, machineId string) instance.Instance {
	stateInfo := jujutesting.FakeStateInfo(machineId)
	apiInfo := jujutesting.FakeAPIInfo(machineId)

	series := "series"
	nonce := "fake-nonce"
	cons := constraints.Value{}
	lxc, _, err := s.broker.StartInstance(machineId, nonce, series, cons, stateInfo, apiInfo)
	c.Assert(err, IsNil)
	return lxc
}

func (s *lxcBrokerSuite) TestStartInstance(c *C) {
	machineId := "1/lxc/0"
	lxc := s.startInstance(c, machineId)
	c.Assert(lxc.Id(), Equals, instance.Id("juju-machine-1-lxc-0"))
	c.Assert(s.lxcContainerDir(lxc), IsDirectory)
	s.assertInstances(c, lxc)
}

func (s *lxcBrokerSuite) TestStopInstance(c *C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	lxc2 := s.startInstance(c, "1/lxc/2")

	err := s.broker.StopInstances([]instance.Instance{lxc0})
	c.Assert(err, IsNil)
	s.assertInstances(c, lxc1, lxc2)
	c.Assert(s.lxcContainerDir(lxc0), DoesNotExist)
	c.Assert(s.lxcRemovedContainerDir(lxc0), IsDirectory)

	err = s.broker.StopInstances([]instance.Instance{lxc1, lxc2})
	c.Assert(err, IsNil)
	s.assertInstances(c)
}

func (s *lxcBrokerSuite) TestAllInstances(c *C) {
	lxc0 := s.startInstance(c, "1/lxc/0")
	lxc1 := s.startInstance(c, "1/lxc/1")
	s.assertInstances(c, lxc0, lxc1)

	err := s.broker.StopInstances([]instance.Instance{lxc1})
	c.Assert(err, IsNil)
	lxc2 := s.startInstance(c, "1/lxc/2")
	s.assertInstances(c, lxc0, lxc2)
}

func (s *lxcBrokerSuite) assertInstances(c *C, inst ...instance.Instance) {
	results, err := s.broker.AllInstances()
	c.Assert(err, IsNil)
	testing.MatchInstances(c, results, inst...)
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
	machineId string
	events    chan mock.Event
}

var _ = Suite(&lxcProvisionerSuite{})

func (s *lxcProvisionerSuite) SetUpSuite(c *C) {
	s.CommonProvisionerSuite.SetUpSuite(c)
	s.lxcSuite.SetUpSuite(c)
}

func (s *lxcProvisionerSuite) TearDownSuite(c *C) {
	s.lxcSuite.TearDownSuite(c)
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func (s *lxcProvisionerSuite) SetUpTest(c *C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.lxcSuite.SetUpTest(c)
	// Write the tools file.
	toolsDir := agent.SharedToolsDir(s.DataDir(), version.Current)
	c.Assert(os.MkdirAll(toolsDir, 0755), IsNil)
	urlPath := filepath.Join(toolsDir, "downloaded-url.txt")
	err := ioutil.WriteFile(urlPath, []byte("http://testing.invalid/tools"), 0644)
	c.Assert(err, IsNil)

	// The lxc provisioner actually needs the machine it is being created on
	// to be in state, in order to get the watcher.
	m, err := s.State.AddMachine(config.DefaultSeries, state.JobHostUnits)
	c.Assert(err, IsNil)
	s.machineId = m.Id()

	s.events = make(chan mock.Event, 25)
	s.Factory.AddListener(s.events)
}

func (s *lxcProvisionerSuite) expectStarted(c *C, machine *state.Machine) string {
	event := <-s.events
	c.Assert(event.Action, Equals, mock.Started)
	err := machine.Refresh()
	c.Assert(err, IsNil)
	s.waitInstanceId(c, machine, instance.Id(event.InstanceId))
	return event.InstanceId
}

func (s *lxcProvisionerSuite) expectStopped(c *C, instId string) {
	event := <-s.events
	c.Assert(event.Action, Equals, mock.Stopped)
	c.Assert(event.InstanceId, Equals, instId)
}

func (s *lxcProvisionerSuite) expectNoEvents(c *C) {
	select {
	case event := <-s.events:
		c.Fatalf("unexpected event %#v", event)
	case <-time.After(200 * time.Millisecond):
		return
	}
}

func (s *lxcProvisionerSuite) TearDownTest(c *C) {
	close(s.events)
	s.lxcSuite.TearDownTest(c)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *lxcProvisionerSuite) newLxcProvisioner() *provisioner.Provisioner {
	return provisioner.NewProvisioner(provisioner.LXC, s.State, s.machineId, s.DataDir())
}

func (s *lxcProvisionerSuite) TestProvisionerStartStop(c *C) {
	p := s.newLxcProvisioner()
	c.Assert(p.Stop(), IsNil)
}

func (s *lxcProvisionerSuite) TestDoesNotStartEnvironMachines(c *C) {
	p := s.newLxcProvisioner()
	defer stop(c, p)

	// Check that an instance is not provisioned when the machine is created.
	_, err := s.State.AddMachine(config.DefaultSeries, state.JobHostUnits)
	c.Assert(err, IsNil)

	s.expectNoEvents(c)
}

func (s *lxcProvisionerSuite) addContainer(c *C) *state.Machine {
	params := state.AddMachineParams{
		ParentId:      s.machineId,
		ContainerType: instance.LXC,
		Series:        config.DefaultSeries,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, IsNil)
	return container
}

func (s *lxcProvisionerSuite) TestContainerStartedAndStopped(c *C) {
	p := s.newLxcProvisioner()
	defer stop(c, p)

	container := s.addContainer(c)

	instId := s.expectStarted(c, container)

	// ...and removed, along with the machine, when the machine is Dead.
	c.Assert(container.EnsureDead(), IsNil)
	s.expectStopped(c, instId)
	s.waitRemoved(c, container)
}
