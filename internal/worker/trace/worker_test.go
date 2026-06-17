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
			_ string,
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
					Endpoint:              "https://meshuggah.com",
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
		Clock:    s.clock,
		Logger:   s.logger,
		Enabled:  false,
		Endpoint: "",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
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
	_, ok := tracer.(coretrace.NoopTracer)
	c.Check(ok, tc.IsTrue)
}

func (s *workerSuite) TestControllerTracingConfigReload(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()

	controllerWatcherChanges := make(chan struct{}, 2)
	getConfigCalled := make(chan struct{}, 3)
	tracerStopped := make(chan struct{}, 1)

	currentConfig := RuntimeConfig{
		Enabled:               true,
		Endpoint:              "https://meshuggah.com",
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
	}
	cfgMutex := sync.Mutex{}

	runtimeConfigProvider := testRuntimeConfigProvider{
		getConfig: func(context.Context) (RuntimeConfig, error) {
			cfgMutex.Lock()
			defer cfgMutex.Unlock()
			select {
			case getConfigCalled <- struct{}{}:
			default:
			}
			return currentConfig, nil
		},
		watchConfig: func(context.Context) (watcher.NotifyWatcher, error) {
			return watchertest.NewMockNotifyWatcher(controllerWatcherChanges), nil
		},
	}

	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
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
		RuntimeConfigProvider: runtimeConfigProvider,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	// Startup should read workload tracing config.
	select {
	case <-getConfigCalled:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for initial workload tracing config read")
	}

	// We need to ensure that the tracers are all drained before we call
	// GetTracer again to ensure that the new config is applied.
	s.ensureWorkersKilled(c)

	worker := w
	_, err = worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(atomic.LoadInt64(&s.called), tc.Equals, int64(1))

	cfgMutex.Lock()
	currentConfig = RuntimeConfig{
		Enabled:               true,
		Endpoint:              "https://gojira.com",
		SampleRatio:           defaultOpenTelemetrySampleRatio,
		TailSamplingThreshold: defaultOpenTelemetryTailSamplingThreshold,
	}
	cfgMutex.Unlock()

	select {
	case controllerWatcherChanges <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending watcher event")
	}

	select {
	case <-getConfigCalled:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for updated workload tracing config read")
	}
	select {
	case <-tracerStopped:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for stopped tracer after config update")
	}

	s.ensureWorkersKilled(c)

	_, err = worker.GetTracer(c.Context(), coretrace.Namespace("agent", "anything"))
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
		Clock:    s.clock,
		Logger:   s.logger,
		Enabled:  true,
		Endpoint: "https://meshuggah.com",
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
			atomic.AddInt64(&s.called, 1)
			return s.trackedTracer, nil
		},
		Tag:  names.NewMachineTag("0"),
		Kind: coretrace.KindController,
		RuntimeConfigProvider: testRuntimeConfigProvider{
			getConfig: func(context.Context) (RuntimeConfig, error) {
				return RuntimeConfig{
					Enabled:               true,
					Endpoint:              "https://meshuggah.com",
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
	return w
}

func (s *workerSuite) newControllerWorker(c *tc.C, runtimeConfigProvider RuntimeConfigProvider) worker.Worker {
	w, err := newWorker(WorkerConfig{
		Clock:  s.clock,
		Logger: s.logger,
		NewTracerWorker: func(context.Context, coretrace.TaggedTracerNamespace, string, string, bool, bool, float64, time.Duration, logger.Logger, NewClientFunc) (TrackedTracer, error) {
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
