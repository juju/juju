// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	context "context"
	url "net/url"
	"sync"
	"testing"
	time "time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher/watchertest"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type WorkerSuite struct {
	baseSuite

	controllerNodeService *MockControllerNodeService

	mutex    sync.Mutex
	called   map[string]int
	finished map[string]chan struct{}
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &WorkerSuite{})
}

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
	cfg.ControllerNodeService = nil
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

	watcher := watchertest.NewMockNotifyWatcher(make(<-chan struct{}))
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithNoServers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{})

	apiRemotes, err := w.GetAPIRemotes()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiRemotes, tc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWithNoServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{}, controllernodeerrors.EmptyAPIAddresses)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{})

	apiRemotes, err := w.GetAPIRemotes()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiRemotes, tc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesWhilstMatchingOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
		"0": {
			"10.0.0.0:17070",
		},
	}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	// Machine-0 is the origin, so we should not have any workers.

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{})

	apiRemotes, err := w.GetAPIRemotes()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(apiRemotes, tc.DeepEquals, []RemoteConnection{})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})
	addr := &url.URL{Scheme: "wss", Host: "10.0.0.1:17070"}

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
		"0": {
			"10.0.0.0:17070",
		},
		"1": {
			"10.0.0.1:17070",
		},
	}, nil)

	s.remote.EXPECT().UpdateAddresses([]string{addr.Host}).DoAndReturn(func(s []string) {
		close(done)
	})
	s.remote.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		ch := make(chan api.Connection, 1)
		go func() {
			select {
			case ch <- s.connection:
			case <-ctx.Done():
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

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	s.ensureChanged(c)

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	remotes, err := w.GetAPIRemotes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(remotes, tc.HasLen, 1)

	var conn api.Connection
	err = remotes[0].Connection(c.Context(), func(ctx context.Context, c api.Connection) error {
		conn = c
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)
	c.Check(conn.Addr(), tc.DeepEquals, addr)

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesUpdatesAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	gomock.InOrder(
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"1": {
				"192.168.0.17",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done1)
		}),
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"1": {
				"192.168.0.18",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.18"}).DoAndReturn(func(s []string) {
			close(done2)
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done1:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done2:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	s.ensureChanged(c)

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})
	c.Check(s.called, tc.DeepEquals, map[string]int{
		"1": 1,
	})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestWorkerAPIServerChangesRemovesOldAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	gomock.InOrder(
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"1": {
				"192.168.0.17",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done1)
		}),
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"2": {
				"192.168.0.18",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.18"}).DoAndReturn(func(s []string) {
			close(done2)
		}),
	)

	s.finished["1"] = make(chan struct{})
	s.finished["2"] = make(chan struct{})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done1:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done2:
	case <-c.Context().Done():
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
	case <-c.Context().Done():
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

	ch := make(chan struct{})
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.controllerNodeService.EXPECT().WatchControllerAPIAddresses(gomock.Any()).Return(watcher, nil)

	done1 := make(chan struct{})
	done2 := make(chan struct{})

	gomock.InOrder(
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"1": {
				"192.168.0.17",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done1)
		}),
		s.controllerNodeService.EXPECT().GetAPIAddressesByControllerIDForAgents(gomock.Any()).Return(map[string][]string{
			"0": {
				"192.168.0.1",
			},
			"1": {
				"192.168.0.17",
			},
		}, nil),
		s.remote.EXPECT().UpdateAddresses([]string{"192.168.0.17"}).DoAndReturn(func(s []string) {
			close(done2)
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done1:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	select {
	case <-done2:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for worker to finish")
	}

	c.Check(w.runner.WorkerNames(), tc.DeepEquals, []string{"1"})
	c.Check(s.called, tc.DeepEquals, map[string]int{"1": 1})

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.controllerNodeService = NewMockControllerNodeService(ctrl)

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
		Origin:                names.NewMachineTag("0"),
		APIInfo:               &api.Info{},
		APIOpener:             api.Open,
		ControllerNodeService: s.controllerNodeService,
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
