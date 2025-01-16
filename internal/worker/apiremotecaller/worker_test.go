// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"sync"
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pubsub/apiserver"
)

type WorkerSuite struct {
	baseSuite

	hub *pubsub.StructuredHub

	mutex    sync.Mutex
	called   map[string]int
	finished map[string]chan struct{}
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestWorkerConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.Origin = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Clock = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Hub = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.APIInfo = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.APIOpener = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewRemote = nil
	c.Assert(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *WorkerSuite) TestWorker(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithNoServers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{})

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{})
	c.Check(w.GetAPIRemotes(), gc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWhilstMatchingOrigin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
		},
	})

	// Machine-0 is the origin, so we should not have any workers.

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{})
	c.Check(w.GetAPIRemotes(), gc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})
	s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
		close(done)
	})
	s.remote.EXPECT().Tag().Return(names.NewMachineTag("1"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"1": {
				ID:              "1",
				Addresses:       []string{"172.217.22.21"},
				InternalAddress: "192.168.0.17",
			},
		},
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-1"})

	remotes := w.GetAPIRemotes()
	c.Assert(remotes, gc.HasLen, 1)
	c.Check(remotes[0].Tag().String(), gc.Equals, "machine-1")

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesNonInternalAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})
	s.remote.EXPECT().UpdateAddresses([]string{"172.217.22.21"}).DoAndReturn(func(s []string) {
		close(done)
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"1": {
				ID:        "1",
				Addresses: []string{"172.217.22.21"},
			},
		},
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-1"})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesRemovesOldAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	gomock.InOrder(
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done1)
		}),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.18"}).DoAndReturn(func(s []string) {
			close(done2)
		}),
	)

	s.finished["machine-1"] = make(chan struct{})
	s.finished["machine-2"] = make(chan struct{})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"1": {
				ID:              "1",
				Addresses:       []string{"172.217.22.21"},
				InternalAddress: "192.168.0.17",
			},
		},
	})

	select {
	case <-done1:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-1"})

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"2": {
				ID:              "2",
				Addresses:       []string{"172.217.22.22"},
				InternalAddress: "192.168.0.18",
			},
		},
	})

	select {
	case <-done2:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	// Wait for the machine-1 to finish, so we can verify that it's been
	// removed.

	select {
	case <-s.finished["machine-1"]:
		// Annoyingly, we need to wait for the worker to finish cleaning
		// up before we can check the worker names. It would be better if
		// runner exposed a way to emit changes to the internal state.
		time.Sleep(100 * time.Millisecond)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-2"})
	c.Check(s.called, jc.DeepEquals, map[string]int{
		"machine-1": 1,
		"machine-2": 1,
	})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithSameAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	gomock.InOrder(
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done1)
		}),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.18"}).DoAndReturn(func(s []string) {
			close(done2)
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"1": {
				ID:              "1",
				Addresses:       []string{"172.217.22.21"},
				InternalAddress: "192.168.0.17",
			},
		},
	})

	select {
	case <-done1:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-1"})

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				ID:              "0",
				Addresses:       []string{"172.217.22.14"},
				InternalAddress: "192.168.0.1",
			},
			"1": {
				ID:              "1",
				Addresses:       []string{"172.217.22.22"},
				InternalAddress: "192.168.0.18",
			},
		},
	})

	select {
	case <-done2:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), gc.DeepEquals, []string{"machine-1"})
	c.Check(s.called, jc.DeepEquals, map[string]int{"machine-1": 1})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	})

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.called = make(map[string]int)

	s.finished = make(map[string]chan struct{})

	return ctrl
}

func (s *WorkerSuite) newWorker(c *gc.C) *remoteWorker {
	w, err := newWorker(s.newConfig(c), s.states)
	c.Assert(err, jc.ErrorIsNil)

	return w
}

func (s *WorkerSuite) newConfig(c *gc.C) WorkerConfig {
	return WorkerConfig{
		Origin:    names.NewMachineTag("0"),
		APIInfo:   &api.Info{},
		APIOpener: api.Open,
		NewRemote: func(rsc RemoteServerConfig) RemoteServer {
			target := rsc.Target.String()

			s.mutex.Lock()
			s.called[target]++
			s.mutex.Unlock()

			once := sync.OnceFunc(func() {
				if finished, ok := s.finished[target]; ok {
					close(finished)
				}
			})

			return newWrappedWorker(s.remote, once)
		},
		Hub:    s.hub,
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

type wrappedWorker struct {
	*MockRemoteServer

	tomb     tomb.Tomb
	finished func()
}

func newWrappedWorker(remote *MockRemoteServer, finished func()) RemoteServer {
	w := &wrappedWorker{
		MockRemoteServer: remote,
		finished:         finished,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

func (w *wrappedWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *wrappedWorker) Wait() error {
	defer w.finished()
	return w.tomb.Wait()
}
