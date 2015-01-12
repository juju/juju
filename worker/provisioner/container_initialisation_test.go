// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/apt"
	"github.com/juju/utils/fslock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/provisioner"
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
	aptCmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	s.aptCmdChan = aptCmdChan

	// Set up provisioner for the state machine.
	s.agentConfig = s.AgentConfigForTag(c, names.NewMachineTag("0"))
	s.p = provisioner.NewEnvironProvisioner(s.provisioner, s.agentConfig)

	// Create a new container initialisation lock.
	s.initLockDir = c.MkDir()
	initLock, err := fslock.NewLock(s.initLockDir, "container-init")
	c.Assert(err, jc.ErrorIsNil)
	s.initLock = initLock
}

func (s *ContainerSetupSuite) TearDownTest(c *gc.C) {
	stop(c, s.p)
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *ContainerSetupSuite) setupContainerWorker(c *gc.C, tag names.MachineTag) (worker.StringsWatchHandler, worker.Runner) {
	testing.PatchExecutable(c, s, "ubuntu-cloudimg-query", containertesting.FakeLxcURLScript)
	runner := worker.NewRunner(allFatal, noImportance)
	pr := s.st.Provisioner()
	machine, err := pr.Machine(tag)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetSupportedContainers(instance.ContainerTypes...)
	c.Assert(err, jc.ErrorIsNil)
	cfg := s.AgentConfigForTag(c, tag)

	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	params := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: instance.ContainerTypes,
		ImageURLGetter:      &containertesting.MockURLGetter{},
		Machine:             machine,
		Provisioner:         pr,
		Config:              cfg,
		InitLock:            s.initLock,
	}
	handler := provisioner.NewContainerSetupHandler(params)
	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return worker.NewStringsWorker(handler), nil
	})
	return handler, runner
}

func (s *ContainerSetupSuite) createContainer(c *gc.C, host *state.Machine, ctype instance.ContainerType) {
	inst := s.checkStartInstanceNoSecureConnection(c, host)
	s.setupContainerWorker(c, host.Tag().(names.MachineTag))

	// make a container on the host machine
	template := state.MachineTemplate{
		Series: coretesting.FakeDefaultSeries,
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := s.State.AddMachineInsideMachine(template, host.Id(), ctype)
	c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetAgentVersion(version.Current)
		c.Assert(err, jc.ErrorIsNil)
		s.assertContainerProvisionerStarted(c, m, ctype)
	}
}

func (s *ContainerSetupSuite) TestLxcContainerUesImageURL(c *gc.C) {
	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, jc.ErrorIsNil)

	brokerCalled := false
	newlxcbroker := func(api provisioner.APICalls, agentConfig agent.Config, managerConfig container.ManagerConfig,
		imageURLGetter container.ImageURLGetter) (environs.InstanceBroker, error) {
		imageURL, err := imageURLGetter.ImageURL(instance.LXC, "trusty", "amd64")
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(imageURL, gc.Equals, "imageURL")
		c.Assert(imageURLGetter.CACert(), gc.DeepEquals, []byte("cert"))
		brokerCalled = true
		return nil, fmt.Errorf("lxc broker error")
	}
	s.PatchValue(&provisioner.NewLxcBroker, newlxcbroker)
	s.createContainer(c, m, instance.LXC)
	c.Assert(brokerCalled, jc.IsTrue)

}

func (s *ContainerSetupSuite) TestContainerManagerConfigName(c *gc.C) {
	pr := s.st.Provisioner()
	expect := func(expect string) {
		cfg, err := provisioner.ContainerManagerConfig(instance.KVM, pr, s.agentConfig)
		c.Assert(err, jc.ErrorIsNil)
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
		Series:      "precise", // precise requires special apt parameters, so we use that series here.
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, jc.ErrorIsNil)

	err = os.RemoveAll(s.initLockDir)
	c.Assert(err, jc.ErrorIsNil)
	handler, runner := s.setupContainerWorker(c, m.Tag().(names.MachineTag))
	runner.Kill()
	err = runner.Wait()
	c.Assert(err, jc.ErrorIsNil)

	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)
	err = handler.Handle([]string{"0/lxc/0"})
	c.Assert(err, gc.ErrorMatches, ".*failed to acquire initialization lock:.*")

}
