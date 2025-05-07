// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpubsub "github.com/juju/juju/internal/pubsub"
	"github.com/juju/juju/internal/pubsub/apiserver"
)

type WorkerSuite struct {
	baseSuite

	hub *pubsub.StructuredHub

	mutex    sync.Mutex
	called   map[string]int
	finished map[string]chan struct{}
}

var _ = tc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestWorkerConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	c.Assert(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.Origin = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Clock = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Hub = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.APIInfo = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.APIOpener = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.NewRemote = nil
	c.Assert(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *WorkerSuite) TestWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithNoServers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{})

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{})
	c.Check(w.GetAPIRemotes(), tc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWhilstMatchingOrigin(c *tc.C) {
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

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{})
	c.Check(w.GetAPIRemotes(), tc.HasLen, 0)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})
	addr := &url.URL{Scheme: "wss", Host: "192.168.0.17"}

	s.remote.EXPECT().UpdateAddresses([]string{addr.Host}).DoAndReturn(func(s []string) {
		close(done)
	})
	s.remote.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		ch := make(chan api.Connection, 1)
		go func() {
			select {
			case ch <- s.connection:
			case <-ctx.Done():
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for connection")
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case conn := <-ch:
			return fn(ctx, conn)
		}
	})
	s.connection.EXPECT().Addr().Return(addr)

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
				InternalAddress: addr.Host,
			},
		},
	})

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	s.ensureChanged(c)

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	remotes := w.GetAPIRemotes()
	c.Assert(remotes, tc.HasLen, 1)

	var conn api.Connection
	err := remotes[0].Connection(context.Background(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)
	c.Check(conn.Addr(), tc.DeepEquals, addr)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesNonInternalAddress(c *tc.C) {
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

	s.ensureChanged(c)

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesRemovesOldAddress(c *tc.C) {
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

	s.finished["1"] = make(chan struct{})
	s.finished["2"] = make(chan struct{})

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

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

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

	// Wait for the 1 to finish, so we can verify that it's been
	// removed.

	select {
	case <-s.finished["1"]:
		// Annoyingly, we need to wait for the worker to finish cleaning
		// up before we can check the worker names. It would be better if
		// runner exposed a way to emit changes to the internal state.
		time.Sleep(100 * time.Millisecond)
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for worker to finish")
	}

	s.ensureChanged(c)

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"2"})
	c.Check(s.called, tc.DeepEquals, map[string]int{
		"1": 1,
		"2": 1,
	})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithSameAddress(c *tc.C) {
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

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

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

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})
	c.Check(s.called, tc.DeepEquals, map[string]int{"1": 1})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.hub = pubsub.NewStructuredHub(&pubsub.StructuredHubConfig{
		Clock:  s.clock,
		Logger: internalpubsub.WrapLogger(loggertesting.WrapCheckLog(c)),
	})

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.called = make(map[string]int)

	s.finished = make(map[string]chan struct{})

	return ctrl
}

func (s *WorkerSuite) newWorker(c *tc.C) *remoteWorker {
	w, err := newWorker(s.newConfig(c), s.states)
	c.Assert(err, tc.ErrorIsNil)

	return w
}

func (s *WorkerSuite) newConfig(c *tc.C) WorkerConfig {
	return WorkerConfig{
		Origin:    names.NewMachineTag("0"),
		APIInfo:   &api.Info{},
		APIOpener: api.Open,
		NewRemote: func(rsc RemoteServerConfig) RemoteServer {
			target := rsc.ControllerID

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
