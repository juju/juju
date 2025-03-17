// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"context"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	changestream "github.com/juju/juju/core/changestream"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	controllerservice "github.com/juju/juju/domain/controller/service"
	domainmodelservice "github.com/juju/juju/domain/model/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&suite{})

type suite struct {
	authority pki.Authority
	testing.IsolationSuite
	workerC                  chan *mockWorker
	providerServicesGetter   modelworkermanager.ProviderServicesGetter
	domainServicesGetter     *MockDomainServicesGetter
	domainServices           *MockDomainServices
	controllerDomainServices *MockControllerDomainServices
	mockModelWatchableService
}

type mockModelWatchableService struct {
	mockModelDeleter   *MockModelDeleter
	mockState          *MockState
	mockWatcherFactory *MockWatcherFactory
	mockStringsWatcher *MockStringsWatcher
}

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
	s.controllerDomainServices = NewMockControllerDomainServices(ctrl)
	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockStringsWatcher = NewMockStringsWatcher(ctrl)

	return ctrl
}

func (s *suite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority
	s.workerC = make(chan *mockWorker, 100)
	s.providerServicesGetter = providerServicesGetter{}
}

func (s *suite) TestStartEmpty(c *gc.C) {

	s.runTest(c, func(_ worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestStartsInitialWorker(c *gc.C) {

	s.runTest(c, func(_ worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		s.assertStarts(c, activatedModelUUID1.String())
	})
}

func (s *suite) TestStartsLaterWorker(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, _ *mockController) {
		changes := make(chan []string, 2)
		changes <- nil
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)

		s.assertStarts(c, activatedModelUUID1.String())
	})
}

func (s *suite) TestStartsMultiple(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUID2 := modeltesting.GenModelUUID(c)
		activatedModelUUID3 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2, activatedModelUUID3}
		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		s.assertStarts(c, activatedModelUUIDsStr...)
	})
}

func (s *suite) TestIgnoresRepetition(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID1, activatedModelUUID1}
		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		s.assertStarts(c, activatedModelUUID1.String())
	})
}

func (s *suite) TestRestartsErrorWorker(c *gc.C) {
	s.runTest(c, func(w worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		workers := s.waitWorkers(c, 1)
		workers[0].tomb.Kill(errors.New("blaf"))

		s.assertStarts(c, activatedModelUUID1.String())
		workertest.CheckAlive(c, w)
	})
}

func (s *suite) TestRestartsFinishedWorker(c *gc.C) {
	// It must be possible to restart the workers for a model due to
	// model migrations: a model can be migrated away from a
	// controller and then migrated back later.
	s.runTest(c, func(w worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		workers := s.waitWorkers(c, 1)
		workertest.CleanKill(c, workers[0])

		s.assertNoWorkers(c)

		changes <- activatedModelUUIDsStr
		workertest.CheckAlive(c, w)
		s.waitWorkers(c, 1)
	})
}

func (s *suite) TestKillsManagers(c *gc.C) {
	s.runTest(c, func(w worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUID2 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		workers := s.waitWorkers(c, 2)
		workertest.CleanKill(c, w)
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestClosedChangesChannel(c *gc.C) {
	s.runDirtyTest(c, func(w worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUID2 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		workers := s.waitWorkers(c, 2)

		close(changes)
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "changes stopped")
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestReport(c *gc.C) {
	s.runTest(c, func(w worker.Worker, _ *mockController) {
		changes := make(chan []string, 1)
		activatedModelUUID1 := modeltesting.GenModelUUID(c)
		activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

		activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
			return uuid.String()
		})
		changes <- activatedModelUUIDsStr
		s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)
		s.assertStarts(c, activatedModelUUID1.String())

		reporter, ok := w.(worker.Reporter)
		c.Assert(ok, jc.IsTrue)
		report := reporter.Report()
		c.Assert(report, gc.NotNil)
		// TODO: pass a clock through in the worker config so it can be passed
		// to the worker.Runner used in the model to control time.
		// For now, we just look at the started state.
		workers := report["workers"].(map[string]any)
		modelWorker := workers[activatedModelUUID1.String()].(map[string]any)
		c.Assert(modelWorker["state"], gc.Equals, "started")
	})
}

type testFunc func(worker.Worker, *mockController)
type killFunc func(*gc.C, worker.Worker)

func (s *suite) runTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.CleanKill, test)
}

func (s *suite) runDirtyTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.DirtyKill, test)
}

type mockControllerState struct{}

func (s mockControllerState) ControllerModelUUID(ctx context.Context) (coremodel.UUID, error) {
	return "", nil
}

func newMockControllerService(s mockControllerState) *controllerservice.Service {
	return controllerservice.NewService(
		s,
	)
}

func newMockControllerState() *mockControllerState {
	return &mockControllerState{}
}

func (s *suite) runKillTest(c *gc.C, kill killFunc, test testFunc) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), gomock.Any()).Return(s.domainServices, nil).AnyTimes()
	s.domainServices.EXPECT().ControllerConfig().AnyTimes()
	mockCtrlState := newMockControllerState()
	s.controllerDomainServices.EXPECT().Controller().Return(newMockControllerService(*mockCtrlState)).AnyTimes()
	domainWatchableService := domainmodelservice.NewWatchableService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
		s.mockWatcherFactory,
	)
	s.mockState.EXPECT().GetAllActivatedModelsUUIDQuery().Return(
		"SELECT uuid from model WHERE activated = true",
	)

	s.domainServices.EXPECT().Model().Return(domainWatchableService).AnyTimes()
	s.mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher("model", changestream.Changed,
		gomock.Any(), gomock.Any()).Return(s.mockStringsWatcher, nil)
	s.mockStringsWatcher.EXPECT().Wait().AnyTimes().Return(nil)

	mockController := newMockController()
	config := modelworkermanager.Config{
		Authority:                s.authority,
		Logger:                   loggertesting.WrapCheckLog(c),
		Controller:               mockController,
		NewModelWorker:           s.startModelWorker,
		ModelMetrics:             dummyModelMetrics{},
		ErrorDelay:               time.Millisecond,
		LogSinkGetter:            dummyLogSinkGetter{logger: c},
		ProviderServicesGetter:   s.providerServicesGetter,
		DomainServicesGetter:     s.domainServicesGetter,
		ControllerDomainServices: s.controllerDomainServices,
		HTTPClientGetter:         stubHTTPclientGetter{},
		GetControllerConfig: func(ctx context.Context, controllerConfigService modelworkermanager.ControllerConfigService) (controller.Config, error) {
			return coretesting.FakeControllerConfig(), nil
		},
	}
	w, err := modelworkermanager.New(config)
	c.Assert(err, jc.ErrorIsNil)
	defer kill(c, w)
	test(w, mockController)
}

type dummyModelMetrics struct{}

func (dummyModelMetrics) ForModel(model names.ModelTag) modelworkermanager.MetricSink {
	return dummyMetricSink{
		Metrics: dependency.DefaultMetrics(),
	}
}

type dummyMetricSink struct {
	dependency.Metrics
}

func (dummyMetricSink) Unregister() bool {
	return true
}

type dummyLogSinkGetter struct {
	corelogger.ModelLogger
	corelogger.LoggerContextGetter

	logger loggertesting.CheckLogger
}

func (l dummyLogSinkGetter) GetLogWriter(ctx context.Context, modelUUID coremodel.UUID) (corelogger.LogWriterCloser, error) {
	return stubLogger{}, nil
}

func (l dummyLogSinkGetter) GetLoggerContext(ctx context.Context, modelUUID coremodel.UUID) (corelogger.LoggerContext, error) {
	return loggertesting.WrapCheckLogForContext(l.logger), nil
}

func (s *suite) startModelWorker(config modelworkermanager.NewModelConfig) (worker.Worker, error) {
	worker := newMockWorker(config)
	s.workerC <- worker
	return worker, nil
}

func (s *suite) assertStarts(c *gc.C, expect ...string) {
	count := len(expect)
	actual := make([]string, count)
	workers := s.waitWorkers(c, count)
	for i, worker := range workers {
		actual[i] = worker.config.ModelUUID
		c.Assert(worker.config.ModelType, gc.Equals, state.ModelTypeIAAS)
	}
	c.Assert(actual, jc.SameContents, expect)
}

func (s *suite) waitWorkers(c *gc.C, expectedCount int) []*mockWorker {
	if expectedCount < 1 {
		c.Fatal("expectedCount must be >= 1")
	}
	workers := make([]*mockWorker, 0, expectedCount)
	for {
		select {
		case worker := <-s.workerC:
			workers = append(workers, worker)
			if len(workers) == expectedCount {
				s.assertNoWorkers(c)
				return workers
			}
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out waiting for workers to be started")
		}
	}
}

func (s *suite) assertNoWorkers(c *gc.C) {
	select {
	case worker := <-s.workerC:
		c.Fatalf("saw unexpected worker: %s", worker.config.ModelUUID)
	case <-time.After(coretesting.ShortWait):
	}
}

func newMockWorker(config modelworkermanager.NewModelConfig) *mockWorker {
	w := &mockWorker{config: config}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

type mockWorker struct {
	tomb   tomb.Tomb
	config modelworkermanager.NewModelConfig
}

func (mock *mockWorker) Kill() {
	mock.tomb.Kill(nil)
}

func (mock *mockWorker) Wait() error {
	return mock.tomb.Wait()
}

type mockController struct {
	testing.Stub
	model mockModel
}

func newMockController() *mockController {
	return &mockController{
		model: mockModel{
			migrationMode: state.MigrationModeNone,
			modelType:     state.ModelTypeIAAS,
		},
	}
}

func (mock *mockController) Config() (controller.Config, error) {
	mock.MethodCall(mock, "Config")
	return make(controller.Config), nil
}

func (mock *mockController) Model(uuid string) (modelworkermanager.Model, func(), error) {
	mock.MethodCall(mock, "Model", uuid)
	if err := mock.NextErr(); err != nil {
		return nil, nil, err
	}
	release := func() {
		mock.MethodCall(mock, "release")
	}
	return &mock.model, release, nil
}

type fakeLogger struct {
	modelworkermanager.RecordLogger
}

func (mock *mockController) RecordLogger(uuid string) (modelworkermanager.RecordLogger, error) {
	mock.MethodCall(mock, "RecordLogger", uuid)
	return &fakeLogger{}, nil
}

type mockModel struct {
	migrationMode state.MigrationMode
	modelType     state.ModelType
}

func (m *mockModel) MigrationMode() state.MigrationMode {
	return m.migrationMode
}

func (m *mockModel) Type() state.ModelType {
	return m.modelType
}

func (m *mockModel) Name() string {
	return "doesn't matter for this test"
}

func (m *mockModel) Owner() names.UserTag {
	return names.NewUserTag("anyone-is-fine")
}
