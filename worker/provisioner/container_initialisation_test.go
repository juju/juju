// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/juju/mutex"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/clock"
	jujuos "github.com/juju/utils/os"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/provisioner"
)

type ContainerSetupSuite struct {
	CommonProvisionerSuite
	p           provisioner.Provisioner
	agentConfig agent.ConfigSetter
	// Record the apt commands issued as part of container initialisation
	aptCmdChan <-chan *exec.Cmd
	lockName   string
}

var _ = gc.Suite(&ContainerSetupSuite{})

func (s *ContainerSetupSuite) SetUpSuite(c *gc.C) {
	// TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping container tests on windows")
	}
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
	aptCmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte{}, nil)
	s.aptCmdChan = aptCmdChan

	// Set up provisioner for the state machine.
	s.agentConfig = s.AgentConfigForTag(c, names.NewMachineTag("0"))
	var err error
	s.p, err = provisioner.NewEnvironProvisioner(s.provisioner, s.agentConfig, s.Environ)
	c.Assert(err, jc.ErrorIsNil)
	s.lockName = "provisioner-test"
}

func (s *ContainerSetupSuite) TearDownTest(c *gc.C) {
	if s.p != nil {
		stop(c, s.p)
	}
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *ContainerSetupSuite) setupContainerWorker(c *gc.C, tag names.MachineTag) (watcher.StringsHandler, worker.Runner) {
	runner := worker.NewRunner(allFatal, noImportance, worker.RestartDelay)
	pr := apiprovisioner.NewState(s.st)
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
		Machine:             machine,
		Provisioner:         pr,
		Config:              cfg,
		InitLockName:        s.lockName,
	}
	handler := provisioner.NewContainerSetupHandler(params)
	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return watcher.NewStringsWorker(watcher.StringsConfig{
			Handler: handler,
		})
	})
	return handler, runner
}

func (s *ContainerSetupSuite) createContainer(c *gc.C, host *state.Machine, ctype instance.ContainerType) {
	inst := s.checkStartInstance(c, host)
	s.setupContainerWorker(c, host.Tag().(names.MachineTag))

	// make a container on the host machine
	template := state.MachineTemplate{
		Series: series.LatestLts(),
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
	s.waitForRemovalMark(c, host)
}

func (s *ContainerSetupSuite) assertContainerProvisionerStarted(
	c *gc.C, host *state.Machine, ctype instance.ContainerType) {

	// A stub worker callback to record what happens.
	var provisionerStarted uint32
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
		c.Assert(containerType, gc.Equals, ctype)
		c.Assert(cfg.Tag(), gc.Equals, host.Tag())
		atomic.StoreUint32(&provisionerStarted, 1)
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	s.createContainer(c, host, ctype)
	// Consume the apt command used to initialise the container.
	select {
	case <-s.aptCmdChan:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("took too long to get command from channel")
	}
	// the container worker should have created the provisioner
	c.Assert(atomic.LoadUint32(&provisionerStarted) > 0, jc.IsTrue)
}

func (s *ContainerSetupSuite) TestContainerProvisionerStarted(c *gc.C) {
	// Specifically ignore LXD here, if present in instance.ContainerTypes.
	containerTypes := []instance.ContainerType{instance.KVM}
	for _, ctype := range containerTypes {
		// create a machine to host the container.
		m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
			Series:      series.LatestLts(),
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: s.defaultConstraints,
		})
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetSupportedContainers(containerTypes)
		c.Assert(err, jc.ErrorIsNil)
		current := version.Binary{
			Number: jujuversion.Current,
			Arch:   arch.HostArch(),
			Series: series.HostSeries(),
		}
		err = m.SetAgentVersion(current)
		c.Assert(err, jc.ErrorIsNil)
		s.assertContainerProvisionerStarted(c, m, ctype)
	}
}

func (s *ContainerSetupSuite) TestKvmContainerUsesHostArch(c *gc.C) {
	// KVM should do what it's told, and use the architecture in
	// constraints.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	s.testContainerConstraintsArch(c, instance.KVM, arch.AMD64)
}

func (s *ContainerSetupSuite) testContainerConstraintsArch(c *gc.C, containerType instance.ContainerType, expectArch string) {
	var called uint32
	s.PatchValue(provisioner.GetToolsFinder, func(*apiprovisioner.State) provisioner.ToolsFinder {
		return toolsFinderFunc(func(v version.Number, series string, arch string) (tools.List, error) {
			atomic.StoreUint32(&called, 1)
			c.Assert(arch, gc.Equals, expectArch)
			result := version.Binary{
				Number: v,
				Arch:   arch,
				Series: series,
			}
			return tools.List{{Version: result}}, nil
		})
	})

	s.PatchValue(&provisioner.StartProvisioner, func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
		toolsFinder.FindTools(jujuversion.Current, series.HostSeries(), arch.AMD64)
		return nil
	})

	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      series.LatestLts(),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{containerType})
	c.Assert(err, jc.ErrorIsNil)
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	err = m.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	s.createContainer(c, m, containerType)
	select {
	case <-s.aptCmdChan:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("took too long to get command from channel")
	}
	c.Assert(atomic.LoadUint32(&called) > 0, jc.IsTrue)
}

func (s *ContainerSetupSuite) TestContainerManagerConfigName(c *gc.C) {
	pr := apiprovisioner.NewState(s.st)
	cfg, err := provisioner.ContainerManagerConfig(instance.KVM, pr, s.agentConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg[container.ConfigModelUUID], gc.Equals, coretesting.ModelTag.Id())
}

type ContainerInstance struct {
	ctype    instance.ContainerType
	packages [][]string
}

func (s *ContainerSetupSuite) assertContainerInitialised(c *gc.C, cont ContainerInstance) {
	// A noop worker callback.
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	current_os, err := series.GetOSFromSeries(series.HostSeries())
	c.Assert(err, jc.ErrorIsNil)

	var ser string
	var expected_initial []string
	switch current_os {
	case jujuos.CentOS:
		ser = "centos7"
		expected_initial = []string{
			"yum", "--assumeyes", "--debuglevel=1", "install"}
	default:
		ser = "precise"
		expected_initial = []string{
			"apt-get", "--option=Dpkg::Options::=--force-confold",
			"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
			"install"}
	}

	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      ser, // precise requires special apt parameters, so we use that series here.
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{instance.LXD, instance.KVM})
	c.Assert(err, jc.ErrorIsNil)
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	err = m.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	s.createContainer(c, m, cont.ctype)

	for _, pack := range cont.packages {
		select {
		case cmd := <-s.aptCmdChan:
			expected := append(expected_initial, pack...)
			c.Assert(cmd.Args, gc.DeepEquals, expected)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("took too long to get command from channel")
		}
	}
}

func (s *ContainerSetupSuite) TestContainerInitialised(c *gc.C) {
	cont, err := getContainerInstance()
	c.Assert(err, jc.ErrorIsNil)

	for _, test := range cont {
		s.assertContainerInitialised(c, test)
	}
}

func (s *ContainerSetupSuite) TestContainerInitLockError(c *gc.C) {
	spec := mutex.Spec{
		Name:  s.lockName,
		Clock: clock.WallClock,
		Delay: coretesting.ShortWait,
	}
	releaser, err := mutex.Acquire(spec)
	c.Assert(err, jc.ErrorIsNil)
	defer releaser.Release()

	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      series.LatestLts(),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.HostSeries(),
	}
	err = m.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	handler, runner := s.setupContainerWorker(c, m.Tag().(names.MachineTag))
	runner.Kill()
	err = runner.Wait()
	c.Assert(err, jc.ErrorIsNil)

	_, err = handler.SetUp()
	c.Assert(err, jc.ErrorIsNil)
	abort := make(chan struct{})
	close(abort)
	err = handler.Handle(abort, []string{"0/lxd/0"})
	c.Assert(err, gc.ErrorMatches, ".*failed to acquire initialization lock:.*")

}

type toolsFinderFunc func(v version.Number, series string, arch string) (tools.List, error)

func (t toolsFinderFunc) FindTools(v version.Number, series string, arch string) (tools.List, error) {
	return t(v, series, arch)
}

func getContainerInstance() (cont []ContainerInstance, err error) {
	cont = []ContainerInstance{
		{instance.KVM, [][]string{
			{"uvtool-libvirt"},
			{"uvtool"},
		}},
	}
	return cont, nil
}
