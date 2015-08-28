// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/fslock"
	"github.com/juju/utils/packaging/manager"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/container"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
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
	fakeLXCNet  string
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
	s.p = provisioner.NewEnvironProvisioner(s.provisioner, s.agentConfig)

	// Create a new container initialisation lock.
	s.initLockDir = c.MkDir()
	initLock, err := fslock.NewLock(s.initLockDir, "container-init")
	c.Assert(err, jc.ErrorIsNil)
	s.initLock = initLock

	// Patch to isolate the test from the host machine.
	s.fakeLXCNet = filepath.Join(c.MkDir(), "lxc-net")
	s.PatchValue(provisioner.EtcDefaultLXCNetPath, s.fakeLXCNet)
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
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
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

func (s *ContainerSetupSuite) TestLxcContainerUsesConstraintsArch(c *gc.C) {
	// LXC should override the architecture in constraints with the
	// host's architecture.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	s.testContainerConstraintsArch(c, instance.LXC, arch.PPC64EL)
}

func (s *ContainerSetupSuite) TestKvmContainerUsesHostArch(c *gc.C) {
	// KVM should do what it's told, and use the architecture in
	// constraints.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	s.testContainerConstraintsArch(c, instance.KVM, arch.AMD64)
}

func (s *ContainerSetupSuite) testContainerConstraintsArch(c *gc.C, containerType instance.ContainerType, expectArch string) {
	var called bool
	s.PatchValue(provisioner.GetToolsFinder, func(*apiprovisioner.State) provisioner.ToolsFinder {
		return toolsFinderFunc(func(v version.Number, series string, arch *string) (tools.List, error) {
			called = true
			c.Assert(arch, gc.NotNil)
			c.Assert(*arch, gc.Equals, expectArch)
			result := version.Current
			result.Number = v
			result.Series = series
			result.Arch = *arch
			return tools.List{{Version: result}}, nil
		})
	})

	s.PatchValue(&provisioner.StartProvisioner, func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
		amd64 := arch.AMD64
		toolsFinder.FindTools(version.Current.Number, version.Current.Series, &amd64)
		return nil
	})

	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      coretesting.FakeDefaultSeries,
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{containerType})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetAgentVersion(version.Current)
	c.Assert(err, jc.ErrorIsNil)

	s.createContainer(c, m, containerType)
	<-s.aptCmdChan
	c.Assert(called, jc.IsTrue)
}

func (s *ContainerSetupSuite) TestLxcContainerUsesImageURL(c *gc.C) {
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
		imageURLGetter container.ImageURLGetter, enableNAT bool, defaultMTU int) (environs.InstanceBroker, error) {
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

func (s *ContainerSetupSuite) assertContainerInitialised(c *gc.C, ctype instance.ContainerType, packages [][]string, addressable bool) {
	// A noop worker callback.
	startProvisionerWorker := func(runner worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder) error {
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

	// Before starting /etc/default/lxc-net should be missing.
	c.Assert(s.fakeLXCNet, jc.DoesNotExist)

	s.createContainer(c, m, ctype)

	// Only feature-flagged addressable containers modify lxc-net.
	if addressable {
		// After initialisation starts, but before running the
		// initializer, lxc-net should be created if ctype is LXC, as the
		// dummy provider supports static address allocation by default.
		if ctype == instance.LXC {
			AssertFileContains(c, s.fakeLXCNet, provisioner.EtcDefaultLXCNet)
			defer os.Remove(s.fakeLXCNet)
		} else {
			c.Assert(s.fakeLXCNet, jc.DoesNotExist)
		}
	}

	for _, pack := range packages {
		cmd := <-s.aptCmdChan
		expected := []string{
			"apt-get", "--option=Dpkg::Options::=--force-confold",
			"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
			"install"}
		expected = append(expected, pack...)
		c.Assert(cmd.Args, gc.DeepEquals, expected)
	}
}

func (s *ContainerSetupSuite) TestContainerInitialised(c *gc.C) {
	for _, test := range []struct {
		ctype    instance.ContainerType
		packages [][]string
	}{
		{instance.LXC, [][]string{
			[]string{"--target-release", "precise-updates/cloud-tools", "lxc"},
			[]string{"--target-release", "precise-updates/cloud-tools", "cloud-image-utils"}}},
		{instance.KVM, [][]string{
			[]string{"uvtool-libvirt"},
			[]string{"uvtool"}}},
	} {
		s.assertContainerInitialised(c, test.ctype, test.packages, false)
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

func (s *ContainerSetupSuite) TestMaybeOverrideDefaultLXCNet(c *gc.C) {
	for i, test := range []struct {
		ctype          instance.ContainerType
		addressable    bool
		expectOverride bool
	}{
		{instance.KVM, false, false},
		{instance.KVM, true, false},
		{instance.LXC, false, false},
		{instance.LXC, true, true}, // the only case when we override; also last
	} {
		c.Logf(
			"test %d: ctype: %q, addressable: %v -> expectOverride: %v",
			i, test.ctype, test.addressable, test.expectOverride,
		)
		err := provisioner.MaybeOverrideDefaultLXCNet(test.ctype, test.addressable)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		if !test.expectOverride {
			c.Check(s.fakeLXCNet, jc.DoesNotExist)
		} else {
			AssertFileContains(c, s.fakeLXCNet, provisioner.EtcDefaultLXCNet)
		}
	}
}

func AssertFileContains(c *gc.C, filename string, expectedContent ...string) {
	// TODO(dimitern): We should put this in juju/testing repo and
	// replace all similar checks with it.
	data, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	for _, s := range expectedContent {
		c.Assert(string(data), jc.Contains, s)
	}
}

func AssertFileContents(c *gc.C, checker gc.Checker, filename string, expectedContent ...string) {
	// TODO(dimitern): We should put this in juju/testing repo and
	// replace all similar checks with it.
	data, err := ioutil.ReadFile(filename)
	c.Assert(err, jc.ErrorIsNil)
	for _, s := range expectedContent {
		c.Assert(string(data), checker, s)
	}
}

type SetIPAndARPForwardingSuite struct {
	coretesting.BaseSuite
}

func (s *SetIPAndARPForwardingSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping for now")
	}
	s.BaseSuite.SetUpSuite(c)
}

var _ = gc.Suite(&SetIPAndARPForwardingSuite{})

func (s *SetIPAndARPForwardingSuite) TestSuccess(c *gc.C) {
	// NOTE: Because PatchExecutableAsEchoArgs does not allow us to
	// assert on earlier invocations of the same binary (each run
	// overwrites the last args used), we only check sysctl was called
	// for the second key (arpProxySysctlKey). We do check the config
	// contains both though.
	fakeConfig := filepath.Join(c.MkDir(), "sysctl.conf")
	testing.PatchExecutableAsEchoArgs(c, s, "sysctl")
	s.PatchValue(provisioner.SysctlConfig, fakeConfig)

	err := provisioner.SetIPAndARPForwarding(true)
	c.Assert(err, jc.ErrorIsNil)
	expectConf := fmt.Sprintf(
		"%s=1\n%s=1",
		provisioner.IPForwardSysctlKey,
		provisioner.ARPProxySysctlKey,
	)
	AssertFileContains(c, fakeConfig, expectConf)
	expectKeyVal := fmt.Sprintf("%s=1", provisioner.IPForwardSysctlKey)
	testing.AssertEchoArgs(c, "sysctl", "-w", expectKeyVal)
	expectKeyVal = fmt.Sprintf("%s=1", provisioner.ARPProxySysctlKey)
	testing.AssertEchoArgs(c, "sysctl", "-w", expectKeyVal)

	err = provisioner.SetIPAndARPForwarding(false)
	c.Assert(err, jc.ErrorIsNil)
	expectConf = fmt.Sprintf(
		"%s=0\n%s=0",
		provisioner.IPForwardSysctlKey,
		provisioner.ARPProxySysctlKey,
	)
	AssertFileContains(c, fakeConfig, expectConf)
	expectKeyVal = fmt.Sprintf("%s=0", provisioner.IPForwardSysctlKey)
	testing.AssertEchoArgs(c, "sysctl", "-w", expectKeyVal)
	expectKeyVal = fmt.Sprintf("%s=0", provisioner.ARPProxySysctlKey)
	testing.AssertEchoArgs(c, "sysctl", "-w", expectKeyVal)
}

func (s *SetIPAndARPForwardingSuite) TestFailure(c *gc.C) {
	fakeConfig := filepath.Join(c.MkDir(), "sysctl.conf")
	testing.PatchExecutableThrowError(c, s, "sysctl", 123)
	s.PatchValue(provisioner.SysctlConfig, fakeConfig)
	expectKeyVal := fmt.Sprintf("%s=1", provisioner.IPForwardSysctlKey)

	err := provisioner.SetIPAndARPForwarding(true)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(
		`cannot set %s: unexpected exit code 123`, expectKeyVal),
	)
	_, err = os.Stat(fakeConfig)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

type toolsFinderFunc func(v version.Number, series string, arch *string) (tools.List, error)

func (t toolsFinderFunc) FindTools(v version.Number, series string, arch *string) (tools.List, error) {
	return t(v, series, arch)
}

// AddressableContainerSetupSuite only contains tests depending on the
// address allocation feature flag being enabled.
type AddressableContainerSetupSuite struct {
	ContainerSetupSuite
}

var _ = gc.Suite(&AddressableContainerSetupSuite{})

func (s *AddressableContainerSetupSuite) enableFeatureFlag() {
	s.SetFeatureFlags(feature.AddressAllocation)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *AddressableContainerSetupSuite) TestContainerInitialised(c *gc.C) {
	for _, test := range []struct {
		ctype    instance.ContainerType
		packages [][]string
	}{
		{instance.LXC, [][]string{{"--target-release", "precise-updates/cloud-tools", "lxc"}, {"--target-release", "precise-updates/cloud-tools", "cloud-image-utils"}}},
		{instance.KVM, [][]string{{"uvtool-libvirt"}, {"uvtool"}}},
	} {
		s.enableFeatureFlag()
		s.assertContainerInitialised(c, test.ctype, test.packages, true)
	}
}

// LXCDefaultMTUSuite only contains tests depending on the
// lxc-default-mtu environment setting being set explicitly.
type LXCDefaultMTUSuite struct {
	ContainerSetupSuite
}

var _ = gc.Suite(&LXCDefaultMTUSuite{})

func (s *LXCDefaultMTUSuite) SetUpTest(c *gc.C) {
	// Explicitly set lxc-default-mtu before JujuConnSuite constructs
	// the environment, as the setting is immutable.
	s.DummyConfig = dummy.SampleConfig()
	s.DummyConfig["lxc-default-mtu"] = 9000
	s.ContainerSetupSuite.SetUpTest(c)
}

func (s *LXCDefaultMTUSuite) TestDefaultMTUPropagatedToNewLXCBroker(c *gc.C) {
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
	newlxcbroker := func(api provisioner.APICalls, agentConfig agent.Config, managerConfig container.ManagerConfig, imageURLGetter container.ImageURLGetter, enableNAT bool, defaultMTU int) (environs.InstanceBroker, error) {
		brokerCalled = true
		c.Assert(defaultMTU, gc.Equals, 9000)
		return nil, fmt.Errorf("lxc broker error")
	}
	s.PatchValue(&provisioner.NewLxcBroker, newlxcbroker)
	s.createContainer(c, m, instance.LXC)
	c.Assert(brokerCalled, jc.IsTrue)
}
