// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/pkg/errors"
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
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/container/testing"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/provisioner"
)

type containerSetupSuite struct {
	coretesting.BaseSuite

	modelUUID      utils.UUID
	controllerUUID utils.UUID

	initialiser  *testing.MockInitialiser
	facadeCaller *apimocks.MockFacadeCaller
	machine      *provisionermocks.MockMachineProvisioner
	notifyWorker *mocks.MockWorker
	manager      *testing.MockManager

	machineLock *fakeMachineLock

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

func (s *containerSetupSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelUUID = utils.MustNewUUID()
	s.controllerUUID = utils.MustNewUUID()

	s.machineLock = &fakeMachineLock{}
	s.done = make(chan struct{})
}

var _ = gc.Suite(&containerSetupSuite{})

func (s *containerSetupSuite) TestStartContainerStartsContainerProvisioner(c *gc.C) {
	defer s.patch(c).Finish()

	// Adding one new container machine.
	s.notify([]string{"0/lxd/0"})

	s.expectContainerManagerConfig("lxd")
	s.initialiser.EXPECT().Initialise().Return(nil)

	s.PatchValue(
		&factory.NewContainerManager,
		func(forType instance.ContainerType, conf container.ManagerConfig) (container.Manager, error) {
			return s.manager, nil
		})

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

func (s *containerSetupSuite) TestContainerManagerConfigError(c *gc.C) {
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

func (s *containerSetupSuite) setUpContainerWorker(c *gc.C) (watcher.StringsHandler, *worker.Runner) {
	runner := worker.NewRunner(worker.RunnerParams{
		IsFatal:       func(_ error) bool { return true },
		MoreImportant: func(_, _ error) bool { return false },
		RestartDelay:  jworker.RestartDelay,
	})

	pState := apiprovisioner.NewStateFromFacade(s.facadeCaller)
	watcherName := fmt.Sprintf("%s-container-watcher", s.machine.Id())

	cfg, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths:             agent.DefaultPaths,
			Tag:               s.machine.MachineTag(),
			UpgradedToVersion: jujuversion.Current,
			Password:          "password",
			Nonce:             "nonce",
			APIAddresses:      []string{"0.0.0.0:12345"},
			CACert:            coretesting.CACert,
			Controller:        names.NewControllerTag(s.controllerUUID.String()),
			Model:             names.NewModelTag(s.modelUUID.String()),
		})
	c.Assert(err, jc.ErrorIsNil)

	args := provisioner.ContainerSetupParams{
		Runner:              runner,
		WorkerName:          watcherName,
		SupportedContainers: instance.ContainerTypes,
		Machine:             s.machine,
		Provisioner:         pState,
		Config:              cfg,
		MachineLock:         s.machineLock,
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

func (s *containerSetupSuite) patch(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.initialiser = testing.NewMockInitialiser(ctrl)
	s.facadeCaller = apimocks.NewMockFacadeCaller(ctrl)
	s.notifyWorker = mocks.NewMockWorker(ctrl)
	s.machine = provisionermocks.NewMockMachineProvisioner(ctrl)
	s.manager = testing.NewMockManager(ctrl)

	s.stubOutProvisioner(ctrl)

	s.machine.EXPECT().Id().Return("0").AnyTimes()
	s.machine.EXPECT().MachineTag().Return(names.NewMachineTag("0")).AnyTimes()

	s.PatchValue(provisioner.GetContainerInitialiser, func(instance.ContainerType, string) container.Initialiser {
		return s.initialiser
	})

	s.manager.EXPECT().ListContainers().Return(nil, nil).AnyTimes()

	return ctrl
}

// stubOutProvisioner is used to effectively ignore provisioner calls that we
// do not care about for testing container provisioning.
// The bulk of the calls mocked here are called in
// authentication.NewAPIAuthenticator, which is passed the provisioner's
// client-side state by the provisioner worker.
func (s *containerSetupSuite) stubOutProvisioner(ctrl *gomock.Controller) {
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
	fExp.FacadeCall("WatchContainersCharmProfiles", gomock.Any(), gomock.Any()).SetArg(2, watchSource).Return(nil).AnyTimes()

	watchOneSource := params.StringsWatchResult{
		StringsWatcherId: "something",
		Changes:          []string{},
	}
	fExp.FacadeCall("WatchModelMachinesCharmProfiles", gomock.Any(), gomock.Any()).SetArg(2, watchOneSource).Return(nil).AnyTimes()

	controllerCfgSource := params.ControllerConfigResult{
		Config: map[string]interface{}{"controller-uuid": s.controllerUUID.String()},
	}
	fExp.FacadeCall("ControllerConfig", nil, gomock.Any()).SetArg(2, controllerCfgSource).Return(nil).AnyTimes()
}

// notify returns a suite behaviour that will cause the upgrade-series watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *containerSetupSuite) notify(messages ...[]string) {
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
func (s *containerSetupSuite) expectContainerManagerConfig(cType instance.ContainerType) {
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
func (s *containerSetupSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
}

type credentialAPIForTest struct{}

func (*credentialAPIForTest) InvalidateModelCredential(reason string) error {
	return nil
}

type fakeMachineLock struct {
	mu sync.Mutex
}

func (f *fakeMachineLock) Acquire(spec machinelock.Spec) (func(), error) {
	f.mu.Lock()
	return func() {
		f.mu.Unlock()
	}, nil
}

func (f *fakeMachineLock) Report(opts ...machinelock.ReportOption) (string, error) {
	return "", nil
}

type fakeWatcher struct {
	worker.Worker
	ch <-chan []string
}

func (w *fakeWatcher) Changes() watcher.StringsChannel {
	return w.ch
}
