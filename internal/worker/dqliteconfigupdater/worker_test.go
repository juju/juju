// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqliteconfigupdater

import (
	"context"
	"sync"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/controllerruntimeconfig"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	jworker "github.com/juju/juju/internal/worker"
)

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &WorkerSuite{})
	})
}

type WorkerSuite struct {
	baseSuite
}

func (s *WorkerSuite) TestConfigValidate(c *tc.C) {
	config := Config{
		ControllerConfigService: s.controllerConfigService,
		ReadRuntimeConfig: func() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
			return controllerruntimeconfig.ControllerRuntimeConfig{}, nil
		},
		ChangeRuntimeConfig: func(controllerruntimeconfig.Mutator) error { return nil },
		Logger:              loggertesting.WrapCheckLog(c),
	}
	c.Check(config.Validate(), tc.ErrorIsNil)

	invalidConfigs := []struct {
		name   string
		mutate func(Config) Config
	}{
		{
			name: "missing controller config service",
			mutate: func(cfg Config) Config {
				cfg.ControllerConfigService = nil
				return cfg
			},
		}, {
			name: "missing read runtime config",
			mutate: func(cfg Config) Config {
				cfg.ReadRuntimeConfig = nil
				return cfg
			},
		}, {
			name: "missing change runtime config",
			mutate: func(cfg Config) Config {
				cfg.ChangeRuntimeConfig = nil
				return cfg
			},
		}, {
			name: "missing logger",
			mutate: func(cfg Config) Config {
				cfg.Logger = nil
				return cfg
			},
		},
	}
	for _, test := range invalidConfigs {
		c.Logf("testing %s", test.name)
		c.Check(test.mutate(config).Validate(), tc.ErrorIs, coreerrors.NotValid)
	}
}

func (s *WorkerSuite) TestNewWorkerValidatesConfig(c *tc.C) {
	w, err := NewWorker(Config{
		ReadRuntimeConfig: func() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
			return controllerruntimeconfig.ControllerRuntimeConfig{}, nil
		},
		ChangeRuntimeConfig: func(controllerruntimeconfig.Mutator) error { return nil },
		Logger:              loggertesting.WrapCheckLog(c),
	})
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// startWorker starts a worker over the given store and returns it with the
// event channel of its mock controller config watcher. The worker's SetUp
// only establishes the watcher subscription; the initial reconciliation
// happens on the first event delivered through the channel.
func (s *WorkerSuite) startWorker(c *tc.C, store *fakeRuntimeConfigStore) (worker.Worker, chan []string) {
	ch := make(chan []string)
	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).
		DoAndReturn(func(context.Context) (watcher.StringsWatcher, error) {
			return watchertest.NewMockStringsWatcher(ch), nil
		})

	w, err := NewWorker(Config{
		ControllerConfigService: s.controllerConfigService,
		ReadRuntimeConfig:       store.ReadRuntimeConfig,
		ChangeRuntimeConfig:     store.ChangeRuntimeConfig,
		Logger:                  loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w, ch
}

func (s *WorkerSuite) expectDqliteConfig(cfg controllerconfigservice.DqliteConfig) {
	s.controllerConfigService.EXPECT().GetDqliteConfig(gomock.Any()).Return(cfg, nil).AnyTimes()
}

func (s *WorkerSuite) TestInitialEventMatchesBaseline(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	s.expectDqliteConfig(dqliteServiceConfig(false, time.Second, time.Second))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	// The watcher's initial all-keys event finds matching values: the
	// baseline is established without a write or a restart.
	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	workertest.CheckAlive(c, w)
	c.Check(store.writeCount(), tc.Equals, 0)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestInitialEventDriftWritesOnceAndRestarts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	// All three Dqlite values drift from the local runtime config.
	s.expectDqliteConfig(dqliteServiceConfig(true, 2*time.Second, 3*time.Second))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	// All changed values are written together, once, and only then is a
	// restart requested.
	<-store.writeCh
	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
	c.Check(err, tc.ErrorMatches, "agent should be restarted: controller config changed: query-tracing-enabled, query-tracing-threshold, dqlite-busy-timeout")

	c.Check(store.readCount(), tc.Equals, 1)
	c.Check(store.writeCount(), tc.Equals, 1)
	c.Check(store.current(), tc.DeepEquals, dqliteRuntimeConfig(true, 2*time.Second, 3*time.Second))
}

func (s *WorkerSuite) TestNoRestartLoopAfterRestart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	s.expectDqliteConfig(dqliteServiceConfig(true, 2*time.Second, 3*time.Second))

	// First process: drift on the initial event, write, restart.
	w, ch := s.startWorker(c, store)
	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh
	<-store.writeCh
	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)

	// Second process (the restarted controller): the same desired
	// values, now matching the local runtime config after the write.
	// Its initial event must reconcile to the baseline without another
	// write or restart.
	w2, ch2 := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w2)

	select {
	case ch2 <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	workertest.CheckAlive(c, w2)
	c.Check(store.writeCount(), tc.Equals, 1)
	workertest.CleanKill(c, w2)
}

func (s *WorkerSuite) TestSubsequentDriftWritesOnceAndRestarts(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	gomock.InOrder(
		s.controllerConfigService.EXPECT().GetDqliteConfig(gomock.Any()).
			Return(dqliteServiceConfig(false, time.Second, time.Second), nil),
		s.controllerConfigService.EXPECT().GetDqliteConfig(gomock.Any()).
			Return(dqliteServiceConfig(true, 2*time.Second, 3*time.Second), nil),
	)

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	// Initial event: matching values, baseline established.
	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh
	workertest.CheckAlive(c, w)
	c.Check(store.writeCount(), tc.Equals, 0)

	// Subsequent event with drifted values: one atomic write of all
	// changed values, then one restart.
	select {
	case ch <- []string{controller.QueryTracingEnabled, controller.QueryTracingThreshold, controller.DqliteBusyTimeout}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh
	<-store.writeCh

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIs, jworker.ErrRestartAgent)
	c.Check(store.writeCount(), tc.Equals, 1)
	c.Check(store.current(), tc.DeepEquals, dqliteRuntimeConfig(true, 2*time.Second, 3*time.Second))
}

func (s *WorkerSuite) TestUnrelatedAndIdenticalEventsDoNothing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	s.expectDqliteConfig(dqliteServiceConfig(false, time.Second, time.Second))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	// Initial event: baseline.
	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	// An event carrying unrelated changed keys: the mutable Dqlite
	// values are re-read, they still match, and no write or restart
	// happens.
	select {
	case ch <- []string{"some-other-config-key"}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh
	workertest.CheckAlive(c, w)

	// A value-identical event for the Dqlite keys changes nothing.
	select {
	case ch <- []string{controller.QueryTracingEnabled}:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	workertest.CheckAlive(c, w)
	c.Check(store.writeCount(), tc.Equals, 0)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestDqliteConfigReadErrorNoRestart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	s.controllerConfigService.EXPECT().GetDqliteConfig(gomock.Any()).
		Return(controllerconfigservice.DqliteConfig{}, errors.New("boom"))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	err := workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "getting Dqlite controller config: boom")
	c.Check(errors.Is(err, jworker.ErrRestartAgent), tc.IsFalse)
	c.Check(store.writeCount(), tc.Equals, 0)
}

func (s *WorkerSuite) TestRuntimeConfigReadErrorNoRestart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	store.setReadErr(errors.New("no such file"))

	s.expectDqliteConfig(dqliteServiceConfig(false, time.Second, time.Second))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}

	err := workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "reading controller runtime config: no such file")
	c.Check(errors.Is(err, jworker.ErrRestartAgent), tc.IsFalse)
	c.Check(store.writeCount(), tc.Equals, 0)
}

func (s *WorkerSuite) TestRuntimeConfigWriteErrorNoRestart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := newFakeRuntimeConfigStore(dqliteRuntimeConfig(false, time.Second, time.Second))
	store.setChangeErr(errors.New("read-only filesystem"))

	s.expectDqliteConfig(dqliteServiceConfig(true, 2*time.Second, 3*time.Second))

	w, ch := s.startWorker(c, store)
	defer workertest.DirtyKill(c, w)

	select {
	case ch <- allControllerConfigKeys:
	case <-c.Context().Done():
		c.Fatalf("event not sent")
	}
	<-store.readCh

	// The write was attempted and failed: the handler returns the
	// underlying failure without wrapping ErrRestartAgent, so no
	// restart is requested.
	err := workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "writing dqlite controller config to controller runtime config: read-only filesystem")
	c.Check(errors.Is(err, jworker.ErrRestartAgent), tc.IsFalse)
	c.Check(store.writeCount(), tc.Equals, 1)
	c.Check(store.current(), tc.DeepEquals, dqliteRuntimeConfig(false, time.Second, time.Second))
}

func (s *WorkerSuite) TestWatchControllerConfigError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).
		Return(nil, errors.New("watch failed"))

	w, err := NewWorker(Config{
		ControllerConfigService: s.controllerConfigService,
		ReadRuntimeConfig: func() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
			return controllerruntimeconfig.ControllerRuntimeConfig{}, nil
		},
		ChangeRuntimeConfig: func(controllerruntimeconfig.Mutator) error { return nil },
		Logger:              loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	err = workertest.CheckKilled(c, w)
	c.Check(err, tc.ErrorMatches, "watch failed")
}

// allControllerConfigKeys mirrors the initial all-keys event emitted by
// WatchControllerConfig.
var allControllerConfigKeys = []string{
	"controller-uuid",
	"ca-cert",
	"api-port",
	controller.QueryTracingEnabled,
	controller.QueryTracingThreshold,
	controller.DqliteBusyTimeout,
}

// dqliteServiceConfig returns the service-level Dqlite tuning values.
func dqliteServiceConfig(enabled bool, threshold, busyTimeout time.Duration) controllerconfigservice.DqliteConfig {
	return controllerconfigservice.DqliteConfig{
		QueryTracingEnabled:   enabled,
		QueryTracingThreshold: threshold,
		DqliteBusyTimeout:     busyTimeout,
	}
}

// dqliteRuntimeConfig returns a controller runtime config carrying the
// local delivery-cache values for the three mutable Dqlite tuning values.
func dqliteRuntimeConfig(enabled bool, threshold, busyTimeout time.Duration) controllerruntimeconfig.ControllerRuntimeConfig {
	return controllerruntimeconfig.ControllerRuntimeConfig{
		QueryTracingEnabled:   enabled,
		QueryTracingThreshold: threshold,
		DqliteBusyTimeout:     busyTimeout,
	}
}

// fakeRuntimeConfigStore is an in-memory runtime config store that records
// reads and writes and can inject read and write failures.
type fakeRuntimeConfigStore struct {
	mu        sync.Mutex
	readErr   error
	changeErr error

	cfg     controllerruntimeconfig.ControllerRuntimeConfig
	reads   int
	writes  int
	readCh  chan struct{}
	writeCh chan struct{}
}

func newFakeRuntimeConfigStore(cfg controllerruntimeconfig.ControllerRuntimeConfig) *fakeRuntimeConfigStore {
	return &fakeRuntimeConfigStore{
		cfg:     cfg,
		readCh:  make(chan struct{}, 8),
		writeCh: make(chan struct{}, 8),
	}
}

func (s *fakeRuntimeConfigStore) setReadErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readErr = err
}

func (s *fakeRuntimeConfigStore) setChangeErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeErr = err
}

func (s *fakeRuntimeConfigStore) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func (s *fakeRuntimeConfigStore) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

func (s *fakeRuntimeConfigStore) current() controllerruntimeconfig.ControllerRuntimeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *fakeRuntimeConfigStore) ReadRuntimeConfig() (controllerruntimeconfig.ControllerRuntimeConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.readErr != nil {
		return controllerruntimeconfig.ControllerRuntimeConfig{}, s.readErr
	}
	cfg := s.cfg
	s.readCh <- struct{}{}
	return cfg, nil
}

func (s *fakeRuntimeConfigStore) ChangeRuntimeConfig(mutate controllerruntimeconfig.Mutator) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.changeErr != nil {
		return s.changeErr
	}
	cfg := s.cfg
	if err := mutate(&cfg); err != nil {
		return err
	}
	s.cfg = cfg
	s.writeCh <- struct{}{}
	return nil
}
