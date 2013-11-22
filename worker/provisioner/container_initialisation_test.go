// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os/exec"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/provisioner"
)

type ContainerSetupSuite struct {
	CommonProvisionerSuite
	p provisioner.Provisioner
	// Record the apt commands issued as part of container initialisation
	aptCmdChan <-chan *exec.Cmd
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
	aptCmdChan, cleanup := testbase.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	s.aptCmdChan = aptCmdChan
	s.AddCleanup(func(*gc.C) { cleanup() })

	// Set up provisioner for the state machine.
	agentConfig := s.AgentConfigForTag(c, "machine-0")
	s.p = provisioner.NewEnvironProvisioner(s.provisioner, agentConfig)
}

func (s *ContainerSetupSuite) TearDownTest(c *gc.C) {
	stop(c, s.p)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *ContainerSetupSuite) setupContainerWorker(c *gc.C, tag string, ctype instance.ContainerType) {
	runner := worker.NewRunner(allFatal, noImportance)
	pr := s.st.Provisioner()
	machine, err := pr.Machine(tag)
	c.Assert(err, gc.IsNil)
	err = machine.AddSupportedContainers(instance.LXC)
	c.Assert(err, gc.IsNil)
	cfg := s.AgentConfigForTag(c, tag)

	watcherName := fmt.Sprintf("%s-watcher", ctype)
	handler := provisioner.NewContainerSetupHandler(runner, watcherName, ctype, machine, pr, cfg)
	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return worker.NewStringsWorker(handler), nil
	})
	go func() {
		runner.Wait()
	}()
}

func (s *ContainerSetupSuite) createContainer(c *gc.C, host *state.Machine, ctype instance.ContainerType) {
	inst := s.checkStartInstance(c, host)
	s.setupContainerWorker(c, host.Tag(), ctype)

	// make a container on the host machine
	params := state.AddMachineParams{
		ParentId:      host.Id(),
		ContainerType: ctype,
		Series:        config.DefaultSeries,
		Jobs:          []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)

	// the host machine agent should not attempt to create the container
	s.checkNoOperations(c)

	// cleanup
	c.Assert(container.EnsureDead(), gc.IsNil)
	c.Assert(container.Remove(), gc.IsNil)
	c.Assert(host.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitRemoved(c, host)
}

func (s *ContainerSetupSuite) assertContainerProvisionerStarted(
	c *gc.C, host *state.Machine, ctype instance.ContainerType) {

	// A stub worker callback to record what happens.
	provisionerStarted := false
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker) error {
		c.Assert(containerType, gc.Equals, ctype)
		c.Assert(cfg.Tag(), gc.Equals, host.Tag())
		provisionerStarted = true
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	s.createContainer(c, host, ctype)
	// Consume the apt command used to initialise the container.
	<-s.aptCmdChan

	// the container worker should have created the provisioner
	c.Assert(provisionerStarted, jc.IsTrue)
}

func (s *ContainerSetupSuite) TestContainerProvisionerStarted(c *gc.C) {
	for _, ctype := range instance.ContainerTypes {
		// create a machine to host the container.
		params := state.AddMachineParams{
			Series:      config.DefaultSeries,
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: s.defaultConstraints,
		}
		m, err := s.BackingState.AddMachineWithConstraints(&params)
		c.Assert(err, gc.IsNil)
		err = m.AddSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
		c.Assert(err, gc.IsNil)
		err = m.SetAgentVersion(version.Current)
		c.Assert(err, gc.IsNil)
		s.assertContainerProvisionerStarted(c, m, ctype)
	}
}

func (s *ContainerSetupSuite) assertContainerInitialised(c *gc.C, ctype instance.ContainerType, packages []string) {
	// A noop worker callback.
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker) error {
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	// create a machine to host the container.
	params := state.AddMachineParams{
		Series:      config.DefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	}
	m, err := s.BackingState.AddMachineWithConstraints(&params)
	c.Assert(err, gc.IsNil)
	err = m.AddSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, gc.IsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)
	s.createContainer(c, m, ctype)

	cmd := <-s.aptCmdChan
	c.Assert(cmd.Env[len(cmd.Env)-1], gc.Equals, "DEBIAN_FRONTEND=noninteractive")
	expected := []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install"}
	expected = append(expected, packages...)
	c.Assert(cmd.Args, gc.DeepEquals, expected)
}

func (s *ContainerSetupSuite) TestContainerInitialised(c *gc.C) {
	for _, test := range []struct {
		ctype    instance.ContainerType
		packages []string
	}{
		{instance.LXC, []string{"lxc"}},
		{instance.KVM, []string{"uvtool-libvirt", "uvtool", "kvm"}},
	} {
		s.assertContainerInitialised(c, test.ctype, test.packages)
	}
}
