// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"

	"github.com/golang/mock/gomock"
	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/packaging/manager"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/common"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	provisionermocks "github.com/juju/juju/api/provisioner/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/testing"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	supportedversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	workercommon "github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/provisioner"
)

type ContainerSetupSuite struct {
	CommonProvisionerSuite
	p           provisioner.Provisioner
	agentConfig agent.ConfigSetter

	modelUUID utils.UUID

	initialiser  *testing.MockInitialiser
	facadeCaller *apimocks.MockFacadeCaller
	machine      *provisionermocks.MockMachineProvisioner
	notifyWorker *mocks.MockWorker

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}

	// Record the apt commands issued as part of container initialisation.
	aptCmdChan  <-chan *exec.Cmd
	machinelock *fakemachinelock
}

var _ = gc.Suite(&ContainerSetupSuite{})

func (s *ContainerSetupSuite) SetUpSuite(c *gc.C) {
	// TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping container tests on windows")
	}
	s.CommonProvisionerSuite.SetUpSuite(c)
}

func (s *ContainerSetupSuite) SetUpTest(c *gc.C) {
	s.CommonProvisionerSuite.SetUpTest(c)
	aptCmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte{}, nil)
	s.aptCmdChan = aptCmdChan

	s.modelUUID = utils.MustNewUUID()

	// Set up provisioner for the state machine.
	s.agentConfig = s.AgentConfigForTag(c, names.NewMachineTag("0"))
	var err error
	s.p, err = provisioner.NewEnvironProvisioner(s.provisioner, s.agentConfig, s.Environ, &credentialAPIForTest{})
	c.Assert(err, jc.ErrorIsNil)
	s.machinelock = &fakemachinelock{}

	s.done = make(chan struct{})
}

func (s *ContainerSetupSuite) TearDownTest(c *gc.C) {
	if s.p != nil {
		workertest.CleanKill(c, s.p)
	}
	s.CommonProvisionerSuite.TearDownTest(c)
}

func (s *ContainerSetupSuite) TestStartContainerStartsContainerProvisioner(c *gc.C) {
	defer s.patch(c).Finish()

	// Adding one new container machine.
	s.notify([]string{"0/lxd/0"})

	s.expectContainerManagerConfig("lxd")
	s.initialiser.EXPECT().Initialise().Return(nil)

	_, runner := s.setUpContainerWorker(c)

	// Watch the runner report. We are waiting for 2 workers to be started:
	// the container watcher and the LXD provisioner.
	workers := make(chan map[string]interface{})
	go func() {
		for {
			rep := runner.Report()["workers"].(map[string]interface{})
			if len(rep) == 2 {
				workers <- rep
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Check that the provisioner is there.
	select {
	case w := <-workers:
		_, ok := w["lxd-provisioner"]
		c.Check(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for runner to start all workers")
	}

	s.cleanKill(c, runner)
}

func (s *ContainerSetupSuite) TestContainerManagerConfigError(c *gc.C) {
	defer s.patch(c).Finish()

	s.facadeCaller.EXPECT().FacadeCall(
		"ContainerManagerConfig", params.ContainerManagerConfigParams{Type: "lxd"}, gomock.Any()).Return(
		errors.New("boom"))

	s.notify(nil)
	handler, runner := s.setUpContainerWorker(c)
	s.cleanKill(c, runner)

	abort := make(chan struct{})
	close(abort)
	err := handler.Handle(abort, []string{"0/lxd/0"})
	c.Assert(err, gc.ErrorMatches, ".*generating container manager config: boom")
}

func (s *ContainerSetupSuite) setUpContainerWorker(c *gc.C) (watcher.StringsHandler, *worker.Runner) {
	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:       func(_ error) bool { return true },
		MoreImportant: func(_, _ error) bool { return false },
		RestartDelay:  jworker.RestartDelay,
	})

	pState := apiprovisioner.NewStateFromFacade(s.facadeCaller)
	watcherName := fmt.Sprintf("%s-container-watcher", s.machine.Id())
	/*
		cfg, err := agent.NewAgentConfig(
			agent.AgentConfigParams{
				Paths:             agent.DefaultPaths,
				Tag:               s.machine.MachineTag(),
				UpgradedToVersion: jujuversion.Current,
				Password:          "password",
				Nonce:             "nonce",
				APIAddresses:      nil,
				CACert:            "",
				Controller:        s.State.ControllerTag(),
				Model:             names.NewModelTag(s.modelUUID.String()),
			})
	*/
	args := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: instance.ContainerTypes,
		Machine:             s.machine,
		Provisioner:         pState,
		Config:              s.AgentConfigForTag(c, s.machine.MachineTag()),
		MachineLock:         s.machinelock,
		CredentialAPI:       &credentialAPIForTest{},
	}

	// Stub out network config getter.
	handler := provisioner.NewContainerSetupHandler(args)
	handler.(*provisioner.ContainerSetup).SetGetNetConfig(
		func(_ common.NetworkConfigSource) ([]params.NetworkConfig, error) {
			return nil, nil
		})

	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return watcher.NewStringsWorker(watcher.StringsConfig{
			Handler: handler,
		})
	})

	return handler, runner
}

func (s *ContainerSetupSuite) setupContainerWorkerOld(c *gc.C, tag names.MachineTag) (watcher.StringsHandler, *worker.Runner) {
	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:       func(_ error) bool { return true },
		MoreImportant: func(_, _ error) bool { return false },
		RestartDelay:  jworker.RestartDelay,
	})
	pr := apiprovisioner.NewState(s.st)
	result, err := pr.Machines(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(result), gc.Equals, 1)
	c.Assert(result[0].Err, gc.IsNil)
	machine := result[0].Machine

	err = machine.SetSupportedContainers(instance.ContainerTypes...)
	c.Assert(err, jc.ErrorIsNil)
	cfg := s.AgentConfigForTag(c, tag)

	watcherName := fmt.Sprintf("%s-container-watcher", machine.Id())
	args := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: instance.ContainerTypes,
		Machine:             machine,
		Provisioner:         pr,
		Config:              cfg,
		MachineLock:         s.machinelock,
		CredentialAPI:       &credentialAPIForTest{},
	}
	handler := provisioner.NewContainerSetupHandler(args)
	handler.(*provisioner.ContainerSetup).SetGetNetConfig(
		func(_ common.NetworkConfigSource) ([]params.NetworkConfig, error) {
			return nil, nil
		})

	runner.StartWorker(watcherName, func() (worker.Worker, error) {
		return watcher.NewStringsWorker(watcher.StringsConfig{
			Handler: handler,
		})
	})
	return handler, runner
}

func (s *ContainerSetupSuite) createContainer(c *gc.C, host *state.Machine, ctype instance.ContainerType) {
	inst := s.checkStartInstance(c, host)
	s.setupContainerWorkerOld(c, host.MachineTag())

	// make a container on the host machine
	template := state.MachineTemplate{
		Series: supportedversion.SupportedLTS(),
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	cntr, err := s.State.AddMachineInsideMachine(template, host.Id(), ctype)
	c.Assert(err, jc.ErrorIsNil)

	// the host machine agent should not attempt to create the container
	s.checkNoOperations(c)

	// cleanup
	c.Assert(cntr.EnsureDead(), gc.IsNil)
	c.Assert(cntr.Remove(), gc.IsNil)
	c.Assert(host.EnsureDead(), gc.IsNil)
	s.checkStopInstances(c, inst)
	s.waitForRemovalMark(c, host)
}

func (s *ContainerSetupSuite) TestKvmContainerUsesTargetArch(c *gc.C) {
	defer s.patch(c).Finish()

	s.initialiser.EXPECT().Initialise().Return(nil)

	// KVM should do what it's told, and use the architecture in constraints.
	s.PatchValue(&arch.HostArch, func() string { return arch.PPC64EL })
	s.testContainerConstraintsArch(c, instance.KVM, arch.AMD64)
}

func (s *ContainerSetupSuite) testContainerConstraintsArch(
	c *gc.C, containerType instance.ContainerType, expectArch string,
) {
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

	s.PatchValue(
		&provisioner.StartProvisioner,
		func(
			runner *worker.Runner,
			containerType instance.ContainerType,
			pr *apiprovisioner.State,
			cfg agent.Config,
			broker environs.InstanceBroker,
			toolsFinder provisioner.ToolsFinder,
			distributionGroupFinder provisioner.DistributionGroupFinder,
			credentialAPI workercommon.CredentialAPI,
		) error {
			toolsFinder.FindTools(jujuversion.Current, series.MustHostSeries(), arch.AMD64)
			return nil
		},
	)

	// create a machine to host the container.
	m, err := s.BackingState.AddOneMachine(state.MachineTemplate{
		Series:      supportedversion.SupportedLTS(),
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: s.defaultConstraints,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetSupportedContainers([]instance.ContainerType{containerType})
	c.Assert(err, jc.ErrorIsNil)
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: series.MustHostSeries(),
	}
	err = m.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	s.createContainer(c, m, containerType)

	c.Assert(atomic.LoadUint32(&called) > 0, jc.IsTrue)
}

type ContainerInstance struct {
	ctype    instance.ContainerType
	packages [][]string
}

func (s *ContainerSetupSuite) assertContainerInitialised(c *gc.C, cont ContainerInstance) {
	// A noop worker callback.
	startProvisionerWorker := func(runner *worker.Runner, containerType instance.ContainerType,
		pr *apiprovisioner.State, cfg agent.Config, broker environs.InstanceBroker,
		toolsFinder provisioner.ToolsFinder, distributionGroupFinder provisioner.DistributionGroupFinder,
		credentialAPI workercommon.CredentialAPI) error {
		return nil
	}
	s.PatchValue(&provisioner.StartProvisioner, startProvisionerWorker)

	currentOs, err := series.GetOSFromSeries(series.MustHostSeries())
	c.Assert(err, jc.ErrorIsNil)

	var ser string
	var expectedInitial []string
	switch currentOs {
	case jujuos.CentOS:
		ser = "centos7"
		expectedInitial = []string{
			"yum", "--assumeyes", "--debuglevel=1", "install"}
	case jujuos.OpenSUSE:
		ser = "opensuseleap"
		expectedInitial = []string{
			"zypper", " --quiet", "--non-interactive-include-reboot-patches", "install"}
	default:
		ser = "precise"
		expectedInitial = []string{
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
		Series: series.MustHostSeries(),
	}
	err = m.SetAgentVersion(current)
	c.Assert(err, jc.ErrorIsNil)

	s.createContainer(c, m, cont.ctype)

	for _, pack := range cont.packages {
		select {
		case cmd := <-s.aptCmdChan:
			expected := append(expectedInitial, pack...)
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

func (s *ContainerSetupSuite) patch(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.initialiser = testing.NewMockInitialiser(ctrl)
	s.facadeCaller = apimocks.NewMockFacadeCaller(ctrl)
	s.notifyWorker = mocks.NewMockWorker(ctrl)
	s.machine = provisionermocks.NewMockMachineProvisioner(ctrl)

	s.stubOutProvisioner(ctrl)

	s.machine.EXPECT().Id().Return("0").AnyTimes()
	s.machine.EXPECT().MachineTag().Return(names.NewMachineTag("0")).AnyTimes()

	s.PatchValue(provisioner.GetContainerInitialiser, func(instance.ContainerType, string) container.Initialiser {
		return s.initialiser
	})

	return ctrl
}

// stubOutProvisioner is used to effectively ignore provisioner calls that we
// do not care about for testing container provisioning.
// The bulk of the calls mocked here are called in
// authentication.NewAPIAuthenticator, which is passed the provisioner's
// client-side state by the provisioner worker.
func (s *ContainerSetupSuite) stubOutProvisioner(ctrl *gomock.Controller) {
	// We could have mocked only the base caller and not the FacadeCaller,
	// but expectations would be verbose to the point of obfuscation.
	// So we only mock the base caller for calls that use it directly,
	// such as watcher acquisition.
	caller := apimocks.NewMockAPICaller(ctrl)
	cExp := caller.EXPECT()
	cExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()
	cExp.APICall("NotifyWatcher", 0, gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(nil).AnyTimes()
	cExp.APICall("StringsWatcher", 0, gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(nil).AnyTimes()

	fExp := s.facadeCaller.EXPECT()
	fExp.RawAPICaller().Return(caller).AnyTimes()

	notifySource := params.NotifyWatchResult{NotifyWatcherId: "who-cares"}
	fExp.FacadeCall("WatchForModelConfigChanges", nil, gomock.Any()).SetArg(2, notifySource).Return(nil).AnyTimes()

	modelCfgSource := params.ModelConfigResult{
		Config: map[string]interface{}{
			"uuid": s.modelUUID.String(),
			"type": "maas",
			"name": "container-init-test-model",
		},
	}
	fExp.FacadeCall("ModelConfig", nil, gomock.Any()).SetArg(2, modelCfgSource).Return(nil).AnyTimes()

	addrSource := params.StringsResult{Result: []string{"0.0.0.0"}}
	fExp.FacadeCall("StateAddresses", nil, gomock.Any()).SetArg(2, addrSource).Return(nil).AnyTimes()
	fExp.FacadeCall("APIAddresses", nil, gomock.Any()).SetArg(2, addrSource).Return(nil).AnyTimes()

	certSource := params.BytesResult{Result: []byte(coretesting.CACert)}
	fExp.FacadeCall("CACert", nil, gomock.Any()).SetArg(2, certSource).Return(nil).AnyTimes()

	uuidSource := params.StringResult{Result: s.modelUUID.String()}
	fExp.FacadeCall("ModelUUID", nil, gomock.Any()).SetArg(2, uuidSource).Return(nil).AnyTimes()

	lifeSource := params.LifeResults{Results: []params.LifeResult{{Life: params.Alive}}}
	fExp.FacadeCall("Life", gomock.Any(), gomock.Any()).SetArg(2, lifeSource).Return(nil).AnyTimes()

	watchSource := params.StringsWatchResults{Results: []params.StringsWatchResult{{
		StringsWatcherId: "whatever",
		Changes:          []string{},
	}}}
	fExp.FacadeCall("WatchContainers", gomock.Any(), gomock.Any()).SetArg(2, watchSource).Return(nil).AnyTimes()

	controllerCfgSource := params.ControllerConfigResult{
		Config: map[string]interface{}{"controller-uuid": utils.MustNewUUID().String()},
	}
	fExp.FacadeCall("ControllerConfig", nil, gomock.Any()).SetArg(2, controllerCfgSource).Return(nil).AnyTimes()
}

// notify returns a suite behaviour that will cause the upgrade-series watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *ContainerSetupSuite) notify(messages ...[]string) {
	ch := make(chan []string)

	go func() {
		for _, m := range messages {
			ch <- m
		}
		close(s.done)
	}()

	s.notifyWorker.EXPECT().Kill().AnyTimes()
	s.notifyWorker.EXPECT().Wait().Return(nil).AnyTimes()

	s.machine.EXPECT().WatchAllContainers().Return(
		&fakeWatcher{
			Worker: s.notifyWorker,
			ch:     ch,
		}, nil)
}

// expectContainerManagerConfig sets up expectations associated with
// acquisition and decoration of container manager configuration.
func (s *ContainerSetupSuite) expectContainerManagerConfig(cType instance.ContainerType) {
	resultSource := params.ContainerManagerConfig{
		ManagerConfig: map[string]string{"model-uuid": s.modelUUID.String()},
	}
	s.facadeCaller.EXPECT().FacadeCall(
		"ContainerManagerConfig", params.ContainerManagerConfigParams{Type: cType}, gomock.Any(),
	).SetArg(2, resultSource).MinTimes(1)

	s.machine.EXPECT().AvailabilityZone().Return("az1", nil)
	s.machine.EXPECT().Series().Return("bionic", nil)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *ContainerSetupSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

type toolsFinderFunc func(v version.Number, series string, arch string) (tools.List, error)

func (t toolsFinderFunc) FindTools(v version.Number, series string, arch string) (tools.List, error) {
	return t(v, series, arch)
}

func getContainerInstance() (cont []ContainerInstance, err error) {
	pkgs := [][]string{
		{"qemu-kvm"},
		{"qemu-utils"},
		{"genisoimage"},
		{"libvirt-bin"},
	}
	if arch.HostArch() == arch.ARM64 {
		pkgs = append([][]string{{"qemu-efi"}}, pkgs...)
	}
	cont = []ContainerInstance{
		{instance.KVM, pkgs},
	}
	return cont, nil
}

type credentialAPIForTest struct{}

func (*credentialAPIForTest) InvalidateModelCredential(reason string) error {
	return nil
}

type fakemachinelock struct {
	mu sync.Mutex
}

func (f *fakemachinelock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

func (f *fakemachinelock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}

type fakeWatcher struct {
	worker.Worker
	ch <-chan []string
}

func (w *fakeWatcher) Changes() watcher.StringsChannel {
	return w.ch
}
