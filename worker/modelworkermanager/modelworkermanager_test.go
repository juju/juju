// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/modelworkermanager"
)

var _ = gc.Suite(&suite{})

type suite struct {
	authority pki.Authority
	testing.IsolationSuite
	workerC chan *mockWorker
}

func (s *suite) SetUpTest(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)
	s.authority = authority
	s.IsolationSuite.SetUpTest(c)
	s.workerC = make(chan *mockWorker, 100)
}

func (s *suite) TestStartEmpty(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, _ *mockController) {
		w.sendModelChange()

		s.assertNoWorkers(c)
	})
}

func (s *suite) TestStartsInitialWorker(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, _ *mockController) {
		w.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestStartsLaterWorker(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, _ *mockController) {
		w.sendModelChange()
		w.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestStartsMultiple(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, _ *mockController) {
		w.sendModelChange("uuid1")
		w.sendModelChange("uuid2", "uuid3")
		w.sendModelChange("uuid4")

		s.assertStarts(c, "uuid1", "uuid2", "uuid3", "uuid4")
	})
}

func (s *suite) TestIgnoresRepetition(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, _ *mockController) {
		w.sendModelChange("uuid")
		w.sendModelChange("uuid", "uuid")
		w.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestRestartsErrorWorker(c *gc.C) {
	s.runTest(c, func(w worker.Worker, mw *mockModelWatcher, _ *mockController) {
		mw.sendModelChange("uuid")
		workers := s.waitWorkers(c, 1)
		workers[0].tomb.Kill(errors.New("blaf"))

		s.assertStarts(c, "uuid")
		workertest.CheckAlive(c, w)
	})
}

func (s *suite) TestRestartsFinishedWorker(c *gc.C) {
	// It must be possible to restart the workers for a model due to
	// model migrations: a model can be migrated away from a
	// controller and then migrated back later.
	s.runTest(c, func(w worker.Worker, mw *mockModelWatcher, _ *mockController) {
		mw.sendModelChange("uuid")
		workers := s.waitWorkers(c, 1)
		workertest.CleanKill(c, workers[0])

		s.assertNoWorkers(c)

		mw.sendModelChange("uuid")
		workertest.CheckAlive(c, w)
		s.waitWorkers(c, 1)
	})
}

func (s *suite) TestKillsManagers(c *gc.C) {
	s.runTest(c, func(w worker.Worker, mw *mockModelWatcher, _ *mockController) {
		mw.sendModelChange("uuid1", "uuid2")
		workers := s.waitWorkers(c, 2)

		workertest.CleanKill(c, w)
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestClosedChangesChannel(c *gc.C) {
	s.runDirtyTest(c, func(w worker.Worker, mw *mockModelWatcher, _ *mockController) {
		mw.sendModelChange("uuid1", "uuid2")
		workers := s.waitWorkers(c, 2)

		close(mw.envWatcher.changes)
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "changes stopped")
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestNoStartingWorkersForImportingModel(c *gc.C) {
	// We shouldn't start workers while the model is importing,
	// otherwise the migrationmaster gets very confused.
	// https://bugs.launchpad.net/juju/+bug/1646310
	s.runTest(c, func(_ worker.Worker, w *mockModelWatcher, g *mockController) {
		g.model.migrationMode = state.MigrationModeImporting
		w.sendModelChange("uuid1")

		s.assertNoWorkers(c)
	})
}

func (s *suite) TestReport(c *gc.C) {
	s.runTest(c, func(w worker.Worker, mw *mockModelWatcher, _ *mockController) {
		mw.sendModelChange("uuid")
		s.assertStarts(c, "uuid")

		reporter, ok := w.(worker.Reporter)
		c.Assert(ok, jc.IsTrue)
		report := reporter.Report()
		c.Assert(report, gc.NotNil)
		// TODO: pass a clock through in the worker config so it can be passed
		// to the worker.Runner used in the model to control time.
		// For now, we just look at the started state.
		workers := report["workers"].(map[string]interface{})
		modelWorker := workers["uuid"].(map[string]interface{})
		c.Assert(modelWorker["state"], gc.Equals, "started")
	})
}

type testFunc func(worker.Worker, *mockModelWatcher, *mockController)
type killFunc func(*gc.C, worker.Worker)

func (s *suite) runTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.CleanKill, test)
}

func (s *suite) runDirtyTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.DirtyKill, test)
}

func (s *suite) runKillTest(c *gc.C, kill killFunc, test testFunc) {
	watcher := newMockModelWatcher()
	controller := newMockController()
	config := modelworkermanager.Config{
		Authority:      s.authority,
		Clock:          clock.WallClock,
		Logger:         loggo.GetLogger("test"),
		MachineID:      "1",
		ModelWatcher:   watcher,
		Controller:     controller,
		NewModelWorker: s.startModelWorker,
		ErrorDelay:     time.Millisecond,
	}
	w, err := modelworkermanager.New(config)
	c.Assert(err, jc.ErrorIsNil)
	defer kill(c, w)
	test(w, watcher, controller)
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

func newMockModelWatcher() *mockModelWatcher {
	return &mockModelWatcher{
		envWatcher: &mockEnvWatcher{
			Worker:  workertest.NewErrorWorker(nil),
			changes: make(chan []string),
		},
	}
}

type mockModelWatcher struct {
	envWatcher *mockEnvWatcher
	modelErr   error
}

func (mock *mockModelWatcher) WatchModels() state.StringsWatcher {
	return mock.envWatcher
}

func (mock *mockModelWatcher) sendModelChange(uuids ...string) {
	mock.envWatcher.changes <- uuids
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
	modelworkermanager.DBLogger
}

func (mock *mockController) DBLogger(uuid string) (modelworkermanager.DBLogger, error) {
	mock.MethodCall(mock, "DBLogger", uuid)
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

type mockEnvWatcher struct {
	worker.Worker
	changes chan []string
}

func (w *mockEnvWatcher) Err() error {
	panic("not used")
}

func (w *mockEnvWatcher) Stop() error {
	return worker.Stop(w)
}

func (w *mockEnvWatcher) Changes() <-chan []string {
	return w.changes
}
