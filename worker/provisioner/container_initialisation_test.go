// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/provisioner"
)

type ContainerSetupSuite struct {
	CommonProvisionerSuite
}

var _ = gc.Suite(&ContainerSetupSuite{})

func (s *ContainerSetupSuite) SetUpSuite(c *gc.C) {
	s.CommonProvisionerSuite.SetUpSuite(c)
}

func (s *ContainerSetupSuite) TearDownSuite(c *gc.C) {
	s.CommonProvisionerSuite.TearDownSuite(c)
}

func allFatal(error) bool {
	return true
}

func noImportance(err0, err1 error) bool {
	return false
}

func (s *ContainerSetupSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	s.CommonProvisionerSuite.setupEnvironmentManager(c)
}

func (s *ContainerSetupSuite) setupContainerWorker(c *gc.C, tag string) {
	runner := worker.NewRunner(allFatal, noImportance)
	pr := s.st.Provisioner()
	machine, err := pr.Machine(tag)
	c.Assert(err, gc.IsNil)
	err = machine.AddSupportedContainers(instance.LXC)
	c.Assert(err, gc.IsNil)
	cfg := s.AgentConfigForTag(c, tag)

	handler := provisioner.NewContainerSetupHandler(runner, "lxc-watcher", instance.LXC, machine, pr, cfg)
	runner.StartWorker("lxc-watcher", func() (worker.Worker, error) {
		return worker.NewStringsWorker(handler), nil
	})
	go func() {
		runner.Wait()
	}()
}

func (s *ContainerSetupSuite) TestContainerProvisionerStarted(c *gc.C) {
	// Set up provisioner for the state machine.
	agentConfig := s.AgentConfigForTag(c, "machine-0")
	p := provisioner.NewProvisioner(provisioner.ENVIRON, s.provisioner, agentConfig)
	defer stop(c, p)

	// create a machine to host the container.
	params := state.AddMachineParams{
		Series:      config.DefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	}
	m, err := s.BackingState.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	inst := s.checkStartInstance(c, m)

	// A stub worker callback to record what happens.
	provisionerStarted := false
	startProvisionerWorker := func(runner worker.Runner, provisionerType provisioner.ProvisionerType,
		pr *apiprovisioner.State, cfg agent.Config) error {
		c.Assert(provisionerType, gc.Equals, provisioner.LXC)
		c.Assert(cfg.Tag(), gc.Equals, m.Tag())
		provisionerStarted = true
		return nil
	}
	provisioner.StartProvisioner = startProvisionerWorker
	s.setupContainerWorker(c, m.Tag())

	// make a container on the machine we just created
	params = state.AddMachineParams{
		ParentId:      m.Id(),
		ContainerType: instance.LXC,
		Series:        config.DefaultSeries,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)

	// the host machine agent should not attempt to create the container
	s.checkNoOperations(c)

	// the container worker should have created the provisioner
	c.Assert(provisionerStarted, jc.IsTrue)

	// cleanup
	c.Assert(container.EnsureDead(), gc.IsNil)
	c.Assert(container.Remove(), gc.IsNil)
	c.Assert(m.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, m)
}
