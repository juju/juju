// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/modelworkermanager"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&suite{})

type suite struct {
	testing.IsolationSuite
	workerC chan *mockWorker
}

func (s *suite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.workerC = make(chan *mockWorker, 100)
}

func (s *suite) TestStartEmpty(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, backend *mockBackend) {
		backend.sendModelChange()

		s.assertNoWorkers(c)
	})
}

func (s *suite) TestStartsInitialWorker(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestStartsLaterWorker(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, backend *mockBackend) {
		backend.sendModelChange()
		backend.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestStartsMultiple(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid1")
		backend.sendModelChange("uuid2", "uuid3")
		backend.sendModelChange("uuid4")

		s.assertStarts(c, "uuid1", "uuid2", "uuid3", "uuid4")
	})
}

func (s *suite) TestIgnoresRepetition(c *gc.C) {
	s.runTest(c, func(_ worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid")
		backend.sendModelChange("uuid", "uuid")
		backend.sendModelChange("uuid")

		s.assertStarts(c, "uuid")
	})
}

func (s *suite) TestRestartsErrorWorker(c *gc.C) {
	s.runTest(c, func(w worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid")
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
	s.runTest(c, func(w worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid")
		workers := s.waitWorkers(c, 1)
		workertest.CleanKill(c, workers[0])

		s.assertNoWorkers(c)

		backend.sendModelChange("uuid")
		workertest.CheckAlive(c, w)
		s.waitWorkers(c, 1)
	})
}

func (s *suite) TestKillsManagers(c *gc.C) {
	s.runTest(c, func(w worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid1", "uuid2")
		workers := s.waitWorkers(c, 2)

		workertest.CleanKill(c, w)
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

func (s *suite) TestClosedChangesChannel(c *gc.C) {
	s.runDirtyTest(c, func(w worker.Worker, backend *mockBackend) {
		backend.sendModelChange("uuid1", "uuid2")
		workers := s.waitWorkers(c, 2)

		close(backend.envWatcher.changes)
		err := workertest.CheckKilled(c, w)
		c.Check(err, gc.ErrorMatches, "changes stopped")
		for _, worker := range workers {
			workertest.CheckKilled(c, worker)
		}
		s.assertNoWorkers(c)
	})
}

type testFunc func(worker.Worker, *mockBackend)
type killFunc func(*gc.C, worker.Worker)

func (s *suite) runTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.CleanKill, test)
}

func (s *suite) runDirtyTest(c *gc.C, test testFunc) {
	s.runKillTest(c, workertest.DirtyKill, test)
}

func (s *suite) runKillTest(c *gc.C, kill killFunc, test testFunc) {
	backend := newMockBackend()
	config := modelworkermanager.Config{
		ControllerUUID: coretesting.ControllerTag.Id(),
		Backend:        backend,
		NewWorker:      s.startModelWorker,
		ErrorDelay:     time.Millisecond,
	}
	w, err := modelworkermanager.New(config)
	c.Assert(err, jc.ErrorIsNil)
	defer kill(c, w)
	test(w, backend)
}

func (s *suite) startModelWorker(controllerUUID, modelUUID string) (worker.Worker, error) {
	worker := newMockWorker(controllerUUID, modelUUID)
	s.workerC <- worker
	return worker, nil
}

func (s *suite) assertStarts(c *gc.C, expect ...string) {
	count := len(expect)
	actual := make([]string, count)
	workers := s.waitWorkers(c, count)
	for i, worker := range workers {
		actual[i] = worker.uuid
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
		c.Fatalf("saw unexpected worker: %s", worker.uuid)
	case <-time.After(coretesting.ShortWait):
	}
}

func newMockWorker(_, modelUUID string) *mockWorker {
	w := &mockWorker{uuid: modelUUID}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w
}

type mockWorker struct {
	tomb tomb.Tomb
	uuid string
}

func (mock *mockWorker) Kill() {
	mock.tomb.Kill(nil)
}

func (mock *mockWorker) Wait() error {
	return mock.tomb.Wait()
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		envWatcher: &mockEnvWatcher{
			Worker:  workertest.NewErrorWorker(nil),
			changes: make(chan []string),
		},
	}
}

type mockBackend struct {
	envWatcher *mockEnvWatcher
}

func (mock *mockBackend) WatchModels() state.StringsWatcher {
	return mock.envWatcher
}

func (mock *mockBackend) sendModelChange(uuids ...string) {
	mock.envWatcher.changes <- uuids
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
