// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner_test

import (
	"context"
	"sync"
	"time"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	provisionermocks "github.com/juju/juju/api/agent/provisioner/mocks"
	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/network"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/container/factory"
	"github.com/juju/juju/internal/container/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/containerprovisioner"
	"github.com/juju/juju/rpc/params"
)

type containerWorkerSuite struct {
	coretesting.BaseSuite

	modelUUID      uuid.UUID
	controllerUUID uuid.UUID

	initialiser    *testing.MockInitialiser
	caller         *apimocks.MockAPICaller
	machine        *provisionermocks.MockMachineProvisioner
	manager        *testing.MockManager
	stringsWatcher *MockStringsWatcher

	machineLock *fakeMachineLock

	// The done channel is used by tests to indicate that
	// the worker has accomplished the scenario and can be stopped.
	done chan struct{}
}

func (s *containerWorkerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelUUID = uuid.MustNewUUID()
	s.controllerUUID = uuid.MustNewUUID()

	s.machineLock = &fakeMachineLock{}
	s.done = make(chan struct{})
}

var _ = gc.Suite(&containerWorkerSuite{})

func (s *containerWorkerSuite) TestContainerSetupAndProvisioner(c *gc.C) {
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

	w := s.setUpContainerWorker(c)
	work, ok := w.(*containerprovisioner.ContainerSetupAndProvisioner)
	c.Assert(ok, jc.IsTrue)

	// Watch the worker report. We are waiting for the lxd-provisioner
	// to be started.
	workers := make(chan struct{}, 1)
	defer close(workers)
	go func() {
		for {
			rep := work.Report()
			if _, ok := rep["lxd-provisioner"].(string); ok {
				workers <- struct{}{}
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Check that the provisioner is there.
	select {
	case _, ok := <-workers:
		c.Check(ok, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for runner to start all workers")
	}

	s.cleanKill(c, w)
}

func (s *containerWorkerSuite) TestContainerSetupAndProvisionerErrWatcherClose(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.initialiser = testing.NewMockInitialiser(ctrl)
	s.caller = apimocks.NewMockAPICaller(ctrl)
	s.caller.EXPECT().BestFacadeVersion("Provisioner").Return(0).AnyTimes()
	s.stringsWatcher = NewMockStringsWatcher(ctrl)
	s.machine = provisionermocks.NewMockMachineProvisioner(ctrl)
	s.manager = testing.NewMockManager(ctrl)
	s.machine.EXPECT().MachineTag().Return(names.NewMachineTag("0")).AnyTimes()

	s.stringsWatcher.EXPECT().Kill().AnyTimes()
	s.stringsWatcher.EXPECT().Wait().AnyTimes()
	s.stringsWatcher.EXPECT().Changes().DoAndReturn(
		func() <-chan []string {
			// Kill the worker while waiting for
			// a container to be provisioned.
			close(s.done)
			return nil
		}).AnyTimes()

	s.PatchValue(
		&factory.NewContainerManager,
		func(forType instance.ContainerType, conf container.ManagerConfig) (container.Manager, error) {
			return s.manager, nil
		})

	w := s.setUpContainerWorker(c)

	s.cleanKill(c, w)
}

func (s *containerWorkerSuite) setUpContainerWorker(c *gc.C) worker.Worker {
	pClient := apiprovisioner.NewClient(s.caller)

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

	args := containerprovisioner.ContainerSetupParams{
		Logger:        loggertesting.WrapCheckLog(c),
		ContainerType: instance.LXD,
		MachineZone:   s.machine,
		MTag:          s.machine.MachineTag(),
		Provisioner:   pClient,
		Config:        cfg,
		MachineLock:   s.machineLock,
		GetNetConfig: func(_ network.ConfigSource) (network.InterfaceInfos, error) {
			return nil, nil
		},
	}
	cs := containerprovisioner.NewContainerSetup(args)

	// Stub out network config getter.
	watcherFunc := func(context.Context) (watcher.StringsWatcher, error) {
		return s.stringsWatcher, nil
	}
	w, err := containerprovisioner.NewContainerSetupAndProvisioner(context.Background(), cs, watcherFunc)
	c.Assert(err, jc.ErrorIsNil)

	return w
}

func (s *containerWorkerSuite) patch(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.initialiser = testing.NewMockInitialiser(ctrl)
	s.caller = apimocks.NewMockAPICaller(ctrl)
	s.caller.EXPECT().BestFacadeVersion("Provisioner").Return(0).AnyTimes()
	s.caller.EXPECT().BestFacadeVersion("NotifyWatcher").Return(0).AnyTimes()
	s.caller.EXPECT().BestFacadeVersion("StringsWatcher").Return(0).AnyTimes()
	s.stringsWatcher = NewMockStringsWatcher(ctrl)
	s.machine = provisionermocks.NewMockMachineProvisioner(ctrl)
	s.manager = testing.NewMockManager(ctrl)

	s.stubOutProvisioner(ctrl)

	s.machine.EXPECT().Id().Return("0").AnyTimes()
	s.machine.EXPECT().MachineTag().Return(names.NewMachineTag("0")).AnyTimes()

	s.PatchValue(containerprovisioner.GetContainerInitialiser, func(instance.ContainerType, map[string]string, containermanager.NetworkingMethod) (container.Initialiser, error) {
		return s.initialiser, nil
	})

	s.manager.EXPECT().ListContainers().Return(nil, nil).AnyTimes()

	return ctrl
}

// stubOutProvisioner is used to effectively ignore provisioner calls that we
// do not care about for testing container provisioning.
// The bulk of the calls mocked here are called in
// authentication.NewAPIAuthenticator, which is passed the provisioner's
// client-side state by the provisioner worker.
func (s *containerWorkerSuite) stubOutProvisioner(ctrl *gomock.Controller) {
	// We could have mocked only the base caller and not the FacadeCaller,
	// but expectations would be verbose to the point of obfuscation.
	// So we only mock the base caller for calls that use it directly,
	// such as watcher acquisition.

	fExp := s.caller.EXPECT()
	fExp.BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()
	fExp.APICall(gomock.Any(), "NotifyWatcher", 0, gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(nil).AnyTimes()
	fExp.APICall(gomock.Any(), "StringsWatcher", 0, gomock.Any(), gomock.Any(), nil, gomock.Any()).Return(nil).AnyTimes()

	notifySource := params.NotifyWatchResult{NotifyWatcherId: "who-cares"}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "WatchForModelConfigChanges", nil, gomock.Any()).SetArg(6, notifySource).Return(nil).AnyTimes()

	modelCfgSource := params.ModelConfigResult{
		Config: map[string]interface{}{
			"uuid":           s.modelUUID.String(),
			"type":           "maas",
			"name":           "container-init-test-model",
			"secret-backend": "auto",
		},
	}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "ModelConfig", nil, gomock.Any()).SetArg(6, modelCfgSource).Return(nil).AnyTimes()

	addrSource := params.StringsResult{Result: []string{"0.0.0.0"}}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "StateAddresses", nil, gomock.Any()).SetArg(6, addrSource).Return(nil).AnyTimes()
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "APIAddresses", nil, gomock.Any()).SetArg(6, addrSource).Return(nil).AnyTimes()

	certSource := params.BytesResult{Result: []byte(coretesting.CACert)}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "CACert", nil, gomock.Any()).SetArg(6, certSource).Return(nil).AnyTimes()

	uuidSource := params.StringResult{Result: s.modelUUID.String()}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "ModelUUID", nil, gomock.Any()).SetArg(6, uuidSource).Return(nil).AnyTimes()

	lifeSource := params.LifeResults{Results: []params.LifeResult{{Life: life.Alive}}}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "Life", gomock.Any(), gomock.Any()).SetArg(6, lifeSource).Return(nil).AnyTimes()

	watchSource := params.StringsWatchResults{Results: []params.StringsWatchResult{{
		StringsWatcherId: "whatever",
		Changes:          []string{},
	}}}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "WatchContainers", gomock.Any(), gomock.Any()).SetArg(6, watchSource).Return(nil).AnyTimes()
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "WatchContainersCharmProfiles", gomock.Any(), gomock.Any()).SetArg(6, watchSource).Return(nil).AnyTimes()

	controllerCfgSource := params.ControllerConfigResult{
		Config: map[string]interface{}{"controller-uuid": s.controllerUUID.String()},
	}
	fExp.APICall(gomock.Any(), "Provisioner", 0, "", "ControllerConfig", nil, gomock.Any()).SetArg(6, controllerCfgSource).Return(nil).AnyTimes()
}

// notify returns a suite behaviour that will cause the container watcher
// to send a number of notifications equal to the supplied argument.
// Once notifications have been consumed, we notify via the suite's channel.
func (s *containerWorkerSuite) notify(messages ...[]string) {
	ch := make(chan []string)

	go func() {
		for _, m := range messages {
			ch <- m
		}
		close(s.done)
	}()

	s.stringsWatcher.EXPECT().Kill().AnyTimes()
	s.stringsWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	s.stringsWatcher.EXPECT().Changes().Return(ch)
}

// expectContainerManagerConfig sets up expectations associated with
// acquisition and decoration of container manager configuration.
func (s *containerWorkerSuite) expectContainerManagerConfig(cType instance.ContainerType) {
	resultSource := params.ContainerManagerConfig{
		ManagerConfig: map[string]string{"model-uuid": s.modelUUID.String()},
	}
	s.caller.EXPECT().APICall(
		gomock.Any(),
		"Provisioner", 0, "", "ContainerManagerConfig", params.ContainerManagerConfigParams{Type: cType}, gomock.Any(),
	).SetArg(6, resultSource).MinTimes(1)

	s.machine.EXPECT().AvailabilityZone(gomock.Any()).Return("az1", nil)
}

// cleanKill waits for notifications to be processed, then waits for the input
// worker to be killed cleanly. If either ops time out, the test fails.
func (s *containerWorkerSuite) cleanKill(c *gc.C, w worker.Worker) {
	select {
	case <-s.done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for notifications to be consumed")
	}
	workertest.CleanKill(c, w)
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
