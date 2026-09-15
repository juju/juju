// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	stdtesting "testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/logger"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	baseSuite

	states        chan string
	trackedTracer *MockTrackedTracer
	called        int64
}

func TestWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &workerSuite{})
	})
}

func (s *workerSuite) TestKilledGetTracerErrDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	w.Kill()

	worker := w.(*tracerWorker)
	_, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIs, coretrace.ErrTracerDying)
}

func (s *workerSuite) TestGetTracer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	tracer, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)

	s.trackedTracer.EXPECT().Start(gomock.Any(), "foo")

	tracer.Start(c.Context(), "foo")

	close(done)
}

func (s *workerSuite) TestGetTracerPassesCACertificate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	const caCertificate = "trace-ca-certificate"
	capturedCACertificate := make(chan string, 1)
	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(
			_ context.Context,
			_ coretrace.TaggedTracerNamespace,
			_, _ string,
			caCertificate string,
			_ bool,
			_ bool,
			_ float64,
			_ time.Duration,
			_ logger.Logger,
			_ NewClientFunc,
		) (TrackedTracer, error) {
			capturedCACertificate <- caCertificate
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindController,
		RuntimeConfigProvider: testRuntimeConfigProvider{
			getConfig: func(context.Context) (RuntimeConfig, error) {
				return RuntimeConfig{
					Enabled:               true,
					HTTPEndpoint:          "https://meshuggah.com",
					CACertificate:         caCertificate,
					SampleRatio:           defaultOpenTelemetrySampleRatio,
					TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
				}, nil
			},
			watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
				return watcher.TODO[struct{}](), nil
			},
		},
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	_, err = w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)

	select {
	case got := <-capturedCACertificate:
		c.Check(got, tc.Equals, caCertificate)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for trace CA certificate")
	}

	close(done)
}

func (s *workerSuite) TestGetTracerIsCached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	for range 10 {
		_, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
		c.Assert(err, tc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetTracerIsNotCachedForDifferentNamespaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	worker := w.(*tracerWorker)
	for i := range 10 {
		_, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", fmt.Sprintf("anything-%d", i)))
		c.Assert(err, tc.ErrorIsNil)
	}

	close(done)

	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))
}

func (s *workerSuite) TestGetTracerConcurrently(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(10)

	worker := w.(*tracerWorker)
	for i := range 10 {
		go func(i int) {
			defer wg.Done()
			_, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", fmt.Sprintf("anything-%d", i)))
			c.Assert(err, tc.ErrorIsNil)
		}(i)
	}

	assertWait(c, wg.Wait)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	close(done)
}

func (s *workerSuite) TestGetTracerDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	w, err := newWorker(WorkerConfig{
		Clock:        s.clock,
		Logger:       s.logger,
		Enabled:      false,
		HTTPEndpoint: "",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindController,
		RuntimeConfigProvider: testRuntimeConfigProvider{
			getConfig: func(context.Context) (RuntimeConfig, error) {
				return RuntimeConfig{}, nil
			},
			watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
				return watcher.TODO[struct{}](), nil
			},
		},
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	worker := w
	tracer, err := worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracer.Enabled(), tc.IsFalse)
}

func (s *workerSuite) TestGetTracerWaitsForInitialRuntimeConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	// Gate the first config read so we can exercise GetTracer while the
	// worker is still waiting for the initial runtime config.
	configReadStarted := make(chan struct{})
	allowConfigRead := make(chan struct{})
	var initialConfigRead sync.Once

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindUnit,
		RuntimeConfigProvider: testRuntimeConfigProvider{
			getConfig: func(context.Context) (RuntimeConfig, error) {
				initialConfigRead.Do(func() {
					close(configReadStarted)
					<-allowConfigRead
				})
				return enabledRuntimeConfig(), nil
			},
			watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
				return watcher.TODO[struct{}](), nil
			},
		},
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	// Wait until the worker has started its first config read (and is blocked).
	select {
	case <-configReadStarted:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for initial runtime config read")
	}

	// GetTracer must fail gracefully while the config read is in-flight.
	cancelledCtx, cancel := context.WithCancel(c.Context())
	cancel()
	tracer, err := w.GetTracer(cancelledCtx, coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIs, context.Canceled)
	c.Check(tracer, tc.IsNil)

	// Unblock the config read and wait for startup to complete.
	close(allowConfigRead)
	s.ensureStartup(c)

	// Now GetTracer should succeed and return an enabled tracer.
	tracer, err = w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracer.Enabled(), tc.IsTrue)
	c.Check(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	close(done)
}

func (s *workerSuite) TestTracerReloadsWhenRuntimeConfigIsEnabled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	// Start with an empty (disabled) config; the fixture drives the watcher.
	fixture := newTestRuntimeConfigFixture(c, 1)

	done := make(chan struct{})
	s.trackedTracer.EXPECT().Kill().AnyTimes()
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	w := s.newControllerWorker(c, fixture.provider()).(*tracerWorker)
	defer workertest.CleanKill(c, w)
	s.ensureStartup(c)

	// The initial config is empty, so the tracer should be disabled.
	tracer, err := w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tracer.Enabled(), tc.IsFalse)

	// Enable tracing via the fixture and notify the watcher.
	fixture.setConfig(enabledRuntimeConfig())
	fixture.sendChange()

	// The tracer is a live view; wait until it picks up the new config.
	waitForCondition(c, tracer.Enabled)

	s.trackedTracer.EXPECT().Start(gomock.Any(), "foo")
	tracer.Start(c.Context(), "foo")
	c.Check(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	close(done)
}

func (s *workerSuite) TestControllerTracingConfigChangeDuringStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	// The fixture returns disabled on the first read and enabled on the
	// second, simulating a config change that arrives during startup.
	fixture := newTestRuntimeConfigFixture(c, 2)
	configReads := 0
	fixture.sequencedConfig = func() RuntimeConfig {
		configReads++
		if configReads == 1 {
			return RuntimeConfig{}
		}
		return enabledRuntimeConfig()
	}

	// Pre-populate the watcher with an event so that it fires as soon as the
	// worker starts processing changes. This simulates a config change that
	// happens between watcher creation and the initial config read.
	fixture.changes <- struct{}{}

	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:                   names.NewMachineTag("0"),
		Kind:                  coretrace.KindController,
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
		RuntimeConfigProvider: fixture.provider(),
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.trackedTracer.EXPECT().Kill().AnyTimes()
	done := make(chan struct{})
	s.trackedTracer.EXPECT().Wait().DoAndReturn(func() error {
		<-done
		return nil
	}).AnyTimes()

	s.ensureStartup(c)

	// Wait for both the initial read and the follow-up read triggered by the
	// watcher event.
	fixture.waitRead(2)

	// The tracer worker is created lazily; poll until it appears.
	waitForCondition(c, func() bool {
		_, err = w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
		c.Assert(err, tc.ErrorIsNil)
		return atomic.LoadInt64(&s.called) > 0
	})
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	close(done)
}

func (s *workerSuite) TestControllerTracingConfigReload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	// The fixture starts enabled; we'll change the endpoint mid-test to
	// verify the worker tears down the old tracer and creates a new one.
	fixture := newTestRuntimeConfigFixture(c, 3)
	fixture.setConfig(enabledRuntimeConfig())

	tracerStopped := make(chan struct{}, 1)
	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return newTrackedTracerStub(func() {
				select {
				case tracerStopped <- struct{}{}:
				default:
				}
			}), nil
		},
		Tag:                   names.NewMachineTag("0"),
		Kind:                  coretrace.KindController,
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
		RuntimeConfigProvider: fixture.provider(),
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	// Startup should read the initial runtime config.
	fixture.waitRead(1)

	_, err = w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	// Change the endpoint and trigger a watcher notification.
	fixture.setConfig(RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "https://gojira.com",
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
	})
	fixture.sendChange()

	// The worker should re-read config and stop the old tracer.
	fixture.waitRead(1)
	select {
	case <-tracerStopped:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for stopped tracer after config update")
	}

	s.ensureWorkersKilled(c)

	// A new tracer should be created on the next GetTracer call.
	_, err = w.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(2))
}

func (s *workerSuite) TestControllerTracingWatcherChannelClosed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	controllerWatcherChanges := make(chan struct{})
	close(controllerWatcherChanges)

	runtimeConfigProvider := testRuntimeConfigProvider{
		getConfig: func(context.Context) (RuntimeConfig, error) {
			return RuntimeConfig{}, nil
		},
		watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(controllerWatcherChanges), nil
		},
	}

	w := s.newControllerWorker(c, runtimeConfigProvider)
	defer workertest.DirtyKill(c, w)

	err := workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorMatches, "runtime config watcher channel closed")
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:        s.clock,
		Logger:       s.logger,
		Enabled:      true,
		HTTPEndpoint: "https://meshuggah.com",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindController,
		RuntimeConfigProvider: testRuntimeConfigProvider{
			getConfig: func(context.Context) (RuntimeConfig, error) {
				return enabledRuntimeConfig(), nil
			},
			watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
				return watcher.TODO[struct{}](), nil
			},
		},
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

func (s *workerSuite) newControllerWorker(c *tc.C, runtimeConfigProvider RuntimeConfigProvider) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:                   names.NewMachineTag("0"),
		Kind:                  coretrace.KindController,
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
		RuntimeConfigProvider: runtimeConfigProvider,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w
}

type testRuntimeConfigProvider struct {
	getConfig   func(context.Context) (RuntimeConfig, error)
	watchConfig func(context.Context) (watcher.NotifyWatcher, error)
}

func (p testRuntimeConfigProvider) CurrentRuntimeConfig(ctx context.Context) (RuntimeConfig, error) {
	if p.getConfig == nil {
		return RuntimeConfig{}, nil
	}
	return p.getConfig(ctx)
}

func (p testRuntimeConfigProvider) WatchRuntimeConfig(ctx context.Context) (watcher.NotifyWatcher, error) {
	if p.watchConfig == nil {
		return nil, nil
	}
	return p.watchConfig(ctx)
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	s.states = make(chan string, 4)
	atomic.StoreInt64(&s.called, 0)

	ctrl := s.baseSuite.setupMocks(c)

	s.trackedTracer = NewMockTrackedTracer(ctrl)
	s.trackedTracer.EXPECT().Enabled().Return(true).AnyTimes()

	return ctrl
}

func (s *workerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *workerSuite) ensureWorkersKilled(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateWorkersKilled)
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for workers killed")
	}
}

func assertWait(c *tc.C, wait func()) {
	done := make(chan struct{})

	go func() {
		defer close(done)
		wait()
	}()

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting")
	}
}

type trackedTracerStub struct {
	coretrace.NoopTracer
	killed chan struct{}
	once   sync.Once
	onKill func()
}

func newTrackedTracerStub(onKill func()) *trackedTracerStub {
	return &trackedTracerStub{
		killed: make(chan struct{}),
		onKill: onKill,
	}
}

func (t *trackedTracerStub) Kill() {
	t.once.Do(func() {
		close(t.killed)
		if t.onKill != nil {
			t.onKill()
		}
	})
}

func (t *trackedTracerStub) Wait() error {
	<-t.killed
	return nil
}

// enabledRuntimeConfig returns a standard RuntimeConfig with tracing enabled.
// This avoids repeating the same struct literal across tests.
func enabledRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Enabled:               true,
		HTTPEndpoint:          "https://meshuggah.com",
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
	}
}

// waitForCondition polls check() until it returns true or the test context is
// cancelled. Use this instead of manual ticker loops.
func waitForCondition(c *tc.C, check func() bool) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for !check() {
		select {
		case <-ticker.C:
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for condition")
		}
	}
}

// testRuntimeConfigFixture bundles mutable runtime config state with the
// synchronisation primitives needed to drive it from tests. It replaces
// the inline mutex + channel + counter setup that was repeated across tests.
type testRuntimeConfigFixture struct {
	c *tc.C

	mu     sync.Mutex
	config RuntimeConfig

	// changes is the watcher notification channel. Send on it to simulate
	// a runtime config change event.
	changes chan struct{}

	// readsCalled is signalled (non-blocking) each time getConfig is invoked.
	// Buffer it to the expected number of reads so the provider never blocks.
	readsCalled chan struct{}

	// sequencedConfig, if non-nil, is called on each getConfig invocation
	// instead of returning the fixed config. Use this when the test needs
	// the provider to return different values on successive reads.
	sequencedConfig func() RuntimeConfig
}

// newTestRuntimeConfigFixture creates a fixture with a watcher change channel
// buffered to readBufSize and a readsCalled channel buffered to readBufSize.
func newTestRuntimeConfigFixture(c *tc.C, readBufSize int) *testRuntimeConfigFixture {
	return &testRuntimeConfigFixture{
		c:           c,
		changes:     make(chan struct{}, readBufSize),
		readsCalled: make(chan struct{}, readBufSize),
	}
}

// setConfig updates the config that the provider returns. Thread-safe.
func (f *testRuntimeConfigFixture) setConfig(cfg RuntimeConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config = cfg
}

// sendChange sends a watcher notification, failing the test on timeout.
func (f *testRuntimeConfigFixture) sendChange() {
	select {
	case f.changes <- struct{}{}:
	case <-f.c.Context().Done():
		f.c.Fatalf("timed out sending watcher change")
	}
}

// waitRead blocks until the provider is invoked (up to n times), failing on
// timeout. Call this before asserting tracer state that depends on config reads.
func (f *testRuntimeConfigFixture) waitRead(n int) {
	for i := range n {
		select {
		case <-f.readsCalled:
		case <-f.c.Context().Done():
			f.c.Fatalf("timed out waiting for config read %d", i+1)
		}
	}
}

// provider returns a testRuntimeConfigProvider wired to this fixture's state.
// Each getConfig invocation signals readsCalled. If sequencedConfig is set it
// takes precedence over the fixed config field.
func (f *testRuntimeConfigFixture) provider() testRuntimeConfigProvider {
	return testRuntimeConfigProvider{
		getConfig: func(context.Context) (RuntimeConfig, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			select {
			case f.readsCalled <- struct{}{}:
			default:
			}
			if f.sequencedConfig != nil {
				return f.sequencedConfig(), nil
			}
			return f.config, nil
		},
		watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(f.changes), nil
		},
	}
}
