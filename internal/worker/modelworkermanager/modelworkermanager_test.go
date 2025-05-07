// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"context"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coretesting "github.com/juju/juju/core/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/services"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/state"
)

var _ = tc.Suite(&suite{})

type suite struct {
	testing.IsolationSuite

	authority pki.Authority
	workerC   chan *mockWorker

	providerServicesGetter modelworkermanager.ProviderServicesGetter

	domainServicesGetter *MockDomainServicesGetter
	domainServices       *MockDomainServices
	modelService         *MockModelService
	leaseManager         *MockManager
}

func (s *suite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)
	s.authority = authority

	s.workerC = make(chan *mockWorker, 100)

	s.providerServicesGetter = providerServicesGetter{}
}

func (s *suite) TestStartEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	changes := make(chan []string, 1)
	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	s.runTest(c, func(_ worker.Worker) {
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestStartsInitialWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

	s.expectServicesForModel(activatedModelUUID1)
	s.expectGetModel(activatedModelUUID1)

	activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
		return uuid.String()
	})

	s.runTest(c, func(_ worker.Worker) {
		select {
		case changes <- activatedModelUUIDsStr:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		s.assertStarts(c, activatedModelUUID1.String())
	})
}

func (s *suite) TestStartsLaterWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 2)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	select {
	case changes <- nil:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out sending changes")
	}

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1}

	s.expectServicesForModel(activatedModelUUID1)
	s.expectGetModel(activatedModelUUID1)

	activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
		return uuid.String()
	})
	s.runTest(c, func(_ worker.Worker) {
		select {
		case changes <- activatedModelUUIDsStr:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		s.assertStarts(c, activatedModelUUID1.String())
	})
}

func (s *suite) TestStartsMultiple(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	var activatedModelUUIDs []string
	for i := 0; i < 3; i++ {
		uuid := modeltesting.GenModelUUID(c)

		activatedModelUUIDs = append(activatedModelUUIDs, uuid.String())

		s.expectServicesForModel(uuid)
		s.expectGetModel(uuid)
	}

	s.runTest(c, func(_ worker.Worker) {
		select {
		case changes <- activatedModelUUIDs:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		s.assertStarts(c, activatedModelUUIDs...)
	})
}

func (s *suite) TestIgnoresRepetition(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	var activatedModelUUIDs []string
	for i := 0; i < 3; i++ {
		uuid := modeltesting.GenModelUUID(c)

		activatedModelUUIDs = append(activatedModelUUIDs, uuid.String())

		s.expectServicesForModel(uuid)
		s.expectGetModel(uuid)
	}

	// Duplicate the first model uuid and add it to the tail.
	activatedModelUUIDs = append(activatedModelUUIDs, activatedModelUUIDs[0])
	s.expectServicesForModel(coremodel.UUID(activatedModelUUIDs[0]))
	s.expectGetModel(coremodel.UUID(activatedModelUUIDs[0]))

	s.runTest(c, func(_ worker.Worker) {
		select {
		case changes <- activatedModelUUIDs:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		s.assertStarts(c, activatedModelUUIDs[0:3]...)
	})
}

func (s *suite) TestRestartsErrorWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	s.expectServicesForModel(activatedModelUUID1)
	s.expectGetModel(activatedModelUUID1)

	s.runTest(c, func(w worker.Worker) {
		select {
		case changes <- []string{activatedModelUUID1.String()}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		workers := s.waitWorkers(c, 1)
		workers[0].tomb.Kill(errors.New("blaf"))

		s.assertStarts(c, activatedModelUUID1.String())

		workertest.CheckAlive(c, w)
	})
}

func (s *suite) TestRestartsFinishedWorker(c *tc.C) {
	// It must be possible to restart the workers for a model due to
	// model migrations: a model can be migrated away from a
	// controller and then migrated back later.
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	s.expectServicesForModelTimes(activatedModelUUID1, 2)
	s.expectGetModelTimes(activatedModelUUID1, 2)

	s.runTest(c, func(w worker.Worker) {
		select {
		case changes <- []string{activatedModelUUID1.String()}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}
		workers := s.waitWorkers(c, 1)
		workertest.CleanKill(c, workers[0])

		s.assertNoWorkers(c)

		select {
		case changes <- []string{activatedModelUUID1.String()}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}
		workertest.CheckAlive(c, w)
		s.waitWorkers(c, 1)
	})
}

func (s *suite) TestKillsManagers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	var activatedModelUUIDs []string
	for i := 0; i < 2; i++ {
		uuid := modeltesting.GenModelUUID(c)

		activatedModelUUIDs = append(activatedModelUUIDs, uuid.String())

		s.expectServicesForModel(uuid)
		s.expectGetModel(uuid)
	}

	s.runTest(c, func(w worker.Worker) {
		select {
		case changes <- activatedModelUUIDs:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}

		workers := s.waitWorkers(c, 2)
		workertest.CleanKill(c, w)
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}

		s.assertNoWorkers(c)
	})
}

func (s *suite) TestClosedChangesChannel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	var activatedModelUUIDs []string
	for i := 0; i < 2; i++ {
		uuid := modeltesting.GenModelUUID(c)

		activatedModelUUIDs = append(activatedModelUUIDs, uuid.String())

		s.expectServicesForModel(uuid)
		s.expectGetModel(uuid)
	}

	s.runDirtyTest(c, func(w worker.Worker) {
		select {
		case changes <- activatedModelUUIDs:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}
		workers := s.waitWorkers(c, 2)

		close(changes)
		err := workertest.CheckKilled(c, w)
		c.Check(err, tc.ErrorMatches, "changes stopped")
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	changes := make(chan []string, 1)

	watcher := watchertest.NewMockStringsWatcher(changes)
	s.modelService.EXPECT().WatchActivatedModels(gomock.Any()).Return(
		watcher, nil,
	)

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	s.expectServicesForModel(activatedModelUUID1)
	s.expectGetModel(activatedModelUUID1)

	s.runTest(c, func(w worker.Worker) {
		select {
		case changes <- []string{activatedModelUUID1.String()}:
		case <-time.After(coretesting.LongWait):
			c.Fatal("timed out sending changes")
		}
		s.assertStarts(c, activatedModelUUID1.String())

		reporter, ok := w.(worker.Reporter)
		c.Assert(ok, tc.IsTrue)
		report := reporter.Report()
		c.Assert(report, tc.NotNil)
		// TODO: pass a clock through in the worker config so it can be passed
		// to the worker.Runner used in the model to control time.
		// For now, we just look at the started state.
		workers := report["workers"].(map[string]any)
		modelWorker := workers[activatedModelUUID1.String()].(map[string]any)
		c.Assert(modelWorker["state"], tc.Equals, "started")
	})
}

type testFunc func(worker.Worker)
type killFunc func(*tc.C, worker.Worker)

func (s *suite) runTest(c *tc.C, test testFunc) {
	s.runKillTest(c, workertest.CleanKill, test)
}

func (s *suite) runDirtyTest(c *tc.C, test testFunc) {
	s.runKillTest(c, workertest.DirtyKill, test)
}

func (s *suite) runKillTest(c *tc.C, kill killFunc, test testFunc) {
	config := modelworkermanager.Config{
		Authority:              s.authority,
		Logger:                 loggertesting.WrapCheckLog(c),
		NewModelWorker:         s.startModelWorker,
		ModelMetrics:           dummyModelMetrics{},
		ErrorDelay:             time.Millisecond,
		LeaseManager:           s.leaseManager,
		LogSinkGetter:          dummyLogSinkGetter{logger: c},
		ProviderServicesGetter: s.providerServicesGetter,
		DomainServicesGetter:   s.domainServicesGetter,
		ModelService:           s.modelService,
		HTTPClientGetter:       stubHTTPClientGetter{},
		GetControllerConfig: func(ctx context.Context, domainServices services.DomainServices) (controller.Config, error) {
			return internaltesting.FakeControllerConfig(), nil
		},
	}
	w, err := modelworkermanager.New(config)
	c.Assert(err, tc.ErrorIsNil)
	defer kill(c, w)
	test(w)
}

func (s *suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.leaseManager = NewMockManager(ctrl)

	return ctrl
}

func (s *suite) expectServicesForModel(modelUUID coremodel.UUID) {
	s.expectServicesForModelTimes(modelUUID, 1)
}

func (s *suite) expectServicesForModelTimes(modelUUID coremodel.UUID, times int) {
	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), modelUUID).Return(s.domainServices, nil).Times(times)
}

func (s *suite) expectGetModel(modelUUID coremodel.UUID) {
	s.expectGetModelTimes(modelUUID, 1)
}

func (s *suite) expectGetModelTimes(modelUUID coremodel.UUID, times int) {
	s.modelService.EXPECT().Model(gomock.Any(), modelUUID).Return(coremodel.Model{
		UUID:      modelUUID,
		ModelType: coremodel.ModelType(state.ModelTypeIAAS),
	}, nil).Times(times)
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

func (l dummyLogSinkGetter) GetLoggerContext(ctx context.Context, modelUUID coremodel.UUID) (corelogger.LoggerContext, error) {
	return loggertesting.WrapCheckLogForContext(l.logger), nil
}

func (s *suite) startModelWorker(config modelworkermanager.NewModelConfig) (worker.Worker, error) {
	worker := newMockWorker(config)
	select {
	case s.workerC <- worker:
	case <-time.After(coretesting.LongWait):
		return nil, errors.New("timed out sending worker")
	}
	return worker, nil
}

func (s *suite) assertStarts(c *tc.C, expect ...string) {
	count := len(expect)
	actual := make([]string, count)
	workers := s.waitWorkers(c, count)
	for i, worker := range workers {
		actual[i] = worker.config.ModelUUID
		c.Assert(worker.config.ModelType, tc.Equals, coremodel.IAAS)
	}
	c.Assert(actual, tc.SameContents, expect)
}

func (s *suite) waitWorkers(c *tc.C, expectedCount int) []*mockWorker {
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

func (s *suite) assertNoWorkers(c *tc.C) {
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
