// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os"
	"os/exec"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils/apt"
	"launchpad.net/juju-core/utils/fslock"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/provisioner"
)

type ContainerSetupSuite struct {
	CommonProvisionerSuite
	p           provisioner.Provisioner
	agentConfig agent.ConfigSetter
	// Record the apt commands issued as part of container initialisation
	aptCmdChan  <-chan *exec.Cmd
	initLockDir string
	initLock    *fslock.Lock
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
	aptCmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	s.aptCmdChan = aptCmdChan

	// Set up provisioner for the state machine.
	s.agentConfig = s.AgentConfigForTag(c, "machine-0")
	s.p = provisioner.NewEnvironProvisioner(s.provisioner, s.agentConfig)

	// Create a new container initialisation lock.
	s.initLockDir = c.MkDir()
	initLock, err := fslock.NewLock(s.initLockDir, "container-init")
	c.Assert(err, gc.IsNil)
	s.initLock = initLock
}

func (s *ContainerSetupSuite) TearDownTest(c *gc.C) {
	stop(c, s.p)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *ContainerSetupSuite) setupContainerWorker(c *gc.C, tag string) worker.StringsWatchHandler {
	runner := worker.NewRunner(allFatal, noImportance)
	pr := s.st.Provisioner()
	machine, err := pr.Machine(tag)
	c.Assert(err, gc.IsNil)
	err = machine.SetSupportedContainers(instance.ContainerTypes...)
	c.Assert(err, gc.IsNil)
	cfg := s.AgentConfigForTag(c, tag)

	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	handler := provisioner.NewContainerSetupHandler(runner, watcherName, instance.ContainerTypes, machine, pr, cfg, s.initLock)
	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return worker.NewStringsWorker(handler), nil
	})
	return handler
}

func (s *ContainerSetupSuite) createContainer(c *gc.C, host *state.Machine, ctype instance.ContainerType) {
	inst := s.checkStartInstance(c, host)
	s.setupContainerWorker(c, host.Tag())

	// make a container on the host machine
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, host.Id(), ctype)
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
		m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
			Series:      coretesting.FakeDefaultSeries,
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: s.defaultConstraints,
		})
		c.Assert(err, gc.IsNil)
		err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
		c.Assert(err, gc.IsNil)
		err = m.SetAgentVersion(version.Current)
		c.Assert(err, gc.IsNil)
		s.assertContainerProvisionerStarted(c, m, ctype)
	}
}

func (s *ContainerSetupSuite) TestContainerManagerConfigName(c *gc.C) {
	pr := s.st.Provisioner()
	expect := func(expect string) {
		cfg, err := provisioner.ContainerManagerConfig(instance.KVM, pr, s.agentConfig)
		c.Assert(err, gc.IsNil)
		c.Assert(cfg[container.ConfigName], gc.Equals, expect)
	}
	expect("juju")
	s.agentConfig.SetValue(agent.Namespace, "any-old-thing")
	expect("any-old-thing")
}

func (s *ContainerSetupSuite) assertContainerInitialised(c *gc.C, ctype instance.ContainerType, packages []string) {
	// A noop worker callback.
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker) error {
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, gc.IsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
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
		{instance.LXC, []string{"--target-release", "precise-updates/cloud-tools", "lxc", "cloud-image-utils"}},
		{instance.KVM, []string{"uvtool-libvirt", "uvtool"}},
	} {
		s.assertContainerInitialised(c, test.ctype, test.packages)
	}
}

func (s *ContainerSetupSuite) TestContainerInitLockError(c *gc.C) {
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, gc.IsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, gc.IsNil)

	err = os.RemoveAll(s.initLockDir)
	c.Assert(err, gc.IsNil)
	handler := s.setupContainerWorker(c, m.Tag())
	_, err = handler.SetUp()
	c.Assert(err, gc.IsNil)
	err = handler.Handle([]string{"0/lxc/0"})
	c.Assert(err, gc.ErrorMatches, ".*failed to acquire initialization lock:.*")

}
