// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	stderrors "errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/semversion"
	internallogger "github.com/juju/juju/internal/logger"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/logsender"
)

type workerSuite struct{}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestStartsLogSinkWhenLokiEndpointEmpty(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 10)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})
}

func (s *workerSuite) TestSwitchStopsOldBackendAndStartsNew(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	fixture.agent.setLokiConfig("http://loki/loki/api/v1/push", "")
	fixture.configChanged.Set(true)

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "routed",
	})
	waitForEvents(c, events, backendEvent{
		backend: "logsink",
		kind:    "stop",
	}, backendEvent{
		backend: "loki",
		kind:    "start",
	})
}

func (s *workerSuite) TestDrainOnlyOverridesEndpoint(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	events := make(chan backendEvent, 10)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		DrainOnly:                 true,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	})
}

func (s *workerSuite) TestBackendFailureFallsBackToDrain(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                failingLogSinkBackendFunc(events),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "buffered",
	})
	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "fallback",
	})

	c.Check(waitForRecord(c, events, "drain-only", "fallback"), tc.DeepEquals, backendEvent{
		backend: "drain-only",
		kind:    "record",
		message: "fallback",
	})
}

func (s *workerSuite) TestBackendStartErrorFallsBackToDrain(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           time.Millisecond * 10,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                errorLogSinkBackendFunc(events),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "buffered",
	})
	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "fallback",
	})

	c.Check(waitForRecord(c, events, "drain-only", "fallback"), tc.DeepEquals, backendEvent{
		backend: "drain-only",
		kind:    "record",
		message: "fallback",
	})
}

func (s *workerSuite) TestBackendRestartRefreshesActiveChannel(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                restartingLogSinkBackendFunc(events),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink-1",
		kind:    "start",
	})
	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "first",
	})
	c.Check(waitForRecord(c, events, "logsink-1", "first"), tc.DeepEquals, backendEvent{
		backend: "logsink-1",
		kind:    "record",
		message: "first",
	})

	// Wait for the failed backend to stop. The restarted backend's "start"
	// event is emitted when logrouter resolves its LogRecords channel, which
	// only happens when we send the next record.
	waitForEvents(c, events, backendEvent{
		backend: "logsink-1",
		kind:    "stop",
	})

	// This loop is necessary because the restarted backend may not be ready to
	// receive the log record immediately after the previous backend has
	// stopped. The restarted backend's LogRecords channel is only resolved when
	// logrouter switches to it, which happens when we send the next record.

	// Ideally, we would use a channel in the runner to know when the backend
	// has been restarted and is ready to receive records, but since we don't
	// have that, we will retry sending the log record until we get a response
	// from the restarted backend.
	for a := internaltesting.LongAttempt.Start(); a.Next(); {
		sendLog(c, fixture.logs, &logsender.LogRecord{
			Time:    time.Now(),
			Module:  "test",
			Level:   loggo.INFO,
			Message: "second",
		})

		if event, ok := waitForRecordWithin(c, events, "logsink-2", "second", time.Millisecond*100); ok {
			c.Check(event, tc.DeepEquals, backendEvent{
				backend: "logsink-2",
				kind:    "record",
				message: "second",
			})
			return
		}
	}
	c.Fatalf("timed out waiting for logsink-2 record %q", "second")
}

func (s *workerSuite) TestReportIncludesActiveBackendAndBackendReports(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 10)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	reporter, ok := w.(interface {
		Report(context.Context) map[string]any
	})
	c.Assert(ok, tc.IsTrue)
	var report map[string]any
	for a := internaltesting.LongAttempt.Start(); a.Next(); {
		report = reporter.Report(c.Context())
		if report["activeBackend"] == "logsink" {
			break
		}
	}
	c.Check(report["activeBackend"], tc.Equals, "logsink")
	c.Check(report["activeBackendID"], tc.Equals, "backend-1")

	backends, ok := report["backends"].(map[string]any)
	c.Assert(ok, tc.IsTrue)
	c.Check(backends["drain"], tc.DeepEquals, map[string]any{
		"name": "drain-only",
	})
	c.Check(backends["backend-1"], tc.DeepEquals, map[string]any{
		"name": "logsink",
	})
}

type fixture struct {
	agent         *testAgent
	logs          logsender.LogRecordCh
	configChanged *voyeur.Value
}

func newFixture(c *tc.C, lokiEndpoint string) fixture {
	cfg, err := agent.NewAgentConfig(agent.AgentConfigParams{
		Paths: agent.Paths{
			DataDir: c.MkDir(),
		},
		Tag:               names.NewMachineTag("0"),
		UpgradedToVersion: semversion.MustParse("4.0.0"),
		Password:          "password",
		CACert:            "ca cert",
		APIAddresses:      []string{"127.0.0.1:17070"},
		Controller:        names.NewControllerTag("01234567-89ab-cdef-0123-456789abcdef"),
		Model:             names.NewModelTag("abcdef01-2345-6789-abcd-ef0123456789"),
	})
	c.Assert(err, tc.ErrorIsNil)
	emptyCACert := ""
	cfg.SetLokiConfig(lokiEndpoint, &emptyCACert, nil)
	return fixture{
		agent: &testAgent{
			cfg: cfg,
		},
		logs:          make(logsender.LogRecordCh, 1),
		configChanged: voyeur.NewValue(false),
	}
}

type testAgent struct {
	mu  sync.Mutex
	cfg agent.ConfigSetterWriter
}

func (a *testAgent) CurrentConfig() agent.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Clone()
}

func (a *testAgent) ChangeConfig(change agent.ConfigMutator) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return change(a.cfg)
}

func (a *testAgent) setLokiConfig(endpoint, caCert string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.SetLokiConfig(endpoint, &caCert, nil)
}

type backendEvent struct {
	backend string
	kind    string
	message string
}

func recordingBackendFunc(events chan<- backendEvent, backendBufferSize int) BackendFunc {
	return func(backendType BackendType, _ ConfigSnapshot) (Backend, error) {
		return newRecordingBackend(string(backendType), events, backendBufferSize), nil
	}
}

func failingLogSinkBackendFunc(events chan<- backendEvent) BackendFunc {
	return func(backendType BackendType, _ ConfigSnapshot) (Backend, error) {
		if backendType != BackendTypeLogSink {
			return newRecordingBackend(string(backendType), events, defaultBackendBufferSize), nil
		}
		w := &failingBackend{
			records: make(logsender.LogRecordCh, 1),
		}
		events <- backendEvent{backend: string(backendType), kind: "start"}
		w.tomb.Go(func() error {
			return stderrors.New("backend failed")
		})
		return w, nil
	}
}

func errorLogSinkBackendFunc(events chan<- backendEvent) BackendFunc {
	return func(backendType BackendType, _ ConfigSnapshot) (Backend, error) {
		if backendType != BackendTypeLogSink {
			return newRecordingBackend(string(backendType), events, defaultBackendBufferSize), nil
		}
		events <- backendEvent{backend: string(backendType), kind: "start"}
		return nil, stderrors.New("backend start failed")
	}
}

func restartingLogSinkBackendFunc(events chan<- backendEvent) BackendFunc {
	var instance int
	return func(backendType BackendType, _ ConfigSnapshot) (Backend, error) {
		if backendType != BackendTypeLogSink {
			return newRecordingBackend(string(backendType), events, defaultBackendBufferSize), nil
		}
		instance++
		return newRestartingBackend(instance, events), nil
	}
}

func newRecordingBackend(name string, events chan<- backendEvent, backendBufferSize int) *recordingBackend {
	w := &recordingBackend{
		name:    name,
		records: make(logsender.LogRecordCh, backendBufferSize),
		events:  events,
	}
	events <- backendEvent{backend: name, kind: "start"}
	w.tomb.Go(w.loop)
	return w
}

type recordingBackend struct {
	tomb     tomb.Tomb
	name     string
	records  logsender.LogRecordCh
	events   chan<- backendEvent
	stopOnce sync.Once
}

func (w *recordingBackend) Kill() {
	w.reportStop()
	w.tomb.Kill(nil)
}

func (w *recordingBackend) Wait() error {
	return w.tomb.Wait()
}

func (w *recordingBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

func (w *recordingBackend) Report(_ context.Context) map[string]any {
	return map[string]any{
		"name": w.name,
	}
}

func (w *recordingBackend) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case rec, ok := <-w.records:
			if !ok {
				w.reportStop()
				return nil
			}
			w.events <- backendEvent{
				backend: w.name,
				kind:    "record",
				message: rec.Message,
			}
		}
	}
}

func (w *recordingBackend) reportStop() {
	w.stopOnce.Do(func() {
		w.events <- backendEvent{backend: w.name, kind: "stop"}
	})
}

type failingBackend struct {
	tomb    tomb.Tomb
	records logsender.LogRecordCh
}

func (w *failingBackend) Kill() {
	w.tomb.Kill(nil)
}

func (w *failingBackend) Wait() error {
	return w.tomb.Wait()
}

func (w *failingBackend) LogRecords() logsender.LogRecordCh {
	return w.records
}

func (w *failingBackend) Report(_ context.Context) map[string]any {
	return nil
}

type restartingBackend struct {
	tomb      tomb.Tomb
	name      string
	records   logsender.LogRecordCh
	events    chan<- backendEvent
	failOnce  bool
	startOnce sync.Once
	stopOnce  sync.Once
}

func newRestartingBackend(instance int, events chan<- backendEvent) *restartingBackend {
	w := &restartingBackend{
		name:     "logsink-" + strconv.Itoa(instance),
		records:  make(logsender.LogRecordCh, 1),
		events:   events,
		failOnce: instance == 1,
	}
	w.tomb.Go(w.loop)
	return w
}

func (w *restartingBackend) Kill() {
	w.reportStop()
	w.tomb.Kill(nil)
}

func (w *restartingBackend) Wait() error {
	return w.tomb.Wait()
}

func (w *restartingBackend) LogRecords() logsender.LogRecordCh {
	w.reportStart()
	return w.records
}

func (w *restartingBackend) Report(_ context.Context) map[string]any {
	return map[string]any{
		"name": w.name,
	}
}

func (w *restartingBackend) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case rec, ok := <-w.records:
			if !ok {
				w.reportStop()
				return nil
			}
			w.events <- backendEvent{
				backend: w.name,
				kind:    "record",
				message: rec.Message,
			}
			if w.failOnce {
				w.reportStop()
				return stderrors.New("backend failed")
			}
		}
	}
}

func (w *restartingBackend) reportStop() {
	w.stopOnce.Do(func() {
		w.events <- backendEvent{backend: w.name, kind: "stop"}
	})
}

func (w *restartingBackend) reportStart() {
	w.startOnce.Do(func() {
		w.events <- backendEvent{backend: w.name, kind: "start"}
	})
}

func waitForEvents(c *tc.C, events <-chan backendEvent, expected ...backendEvent) {
	pending := make(map[backendEvent]struct{}, len(expected))
	for _, event := range expected {
		pending[event] = struct{}{}
	}
	for len(pending) > 0 {
		select {
		case event := <-events:
			delete(pending, event)
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for backend events: %#v", pending)
		}
	}
}

func waitForRecord(c *tc.C, events <-chan backendEvent, backend, message string) backendEvent {
	for {
		select {
		case event := <-events:
			if event.backend == backend && event.kind == "record" && event.message == message {
				return event
			}
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for %s record %q", backend, message)
		}
	}
}

func waitForRecordWithin(
	c *tc.C,
	events <-chan backendEvent,
	backend, message string,
	timeout time.Duration,
) (backendEvent, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case event := <-events:
			if event.backend == backend && event.kind == "record" && event.message == message {
				return event, true
			}
		case <-timer.C:
			return backendEvent{}, false
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for %s record %q", backend, message)
		}
	}
}

func (s *workerSuite) TestManageLegacyLogSinkWriterOnLokiSwitch(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	addCh := make(chan struct{}, 1)
	removeCh := make(chan struct{}, 1)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() { removeCh <- struct{}{} },
		AddLegacyLogSinkWriter:    func() error { addCh <- struct{}{}; return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	// Drain initial callbacks.
	select {
	case <-addCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for initial AddLegacyLogSinkWriter callback")
	}
	drainCh(c, removeCh)

	fixture.agent.setLokiConfig("http://loki/loki/api/v1/push", "")
	fixture.configChanged.Set(true)

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "routed",
	})
	waitForEvents(c, events, backendEvent{
		backend: "logsink",
		kind:    "stop",
	}, backendEvent{
		backend: "loki",
		kind:    "start",
	})

	// Switching to Loki should call RemoveLegacyLogSinkWriter, not Add.
	select {
	case <-removeCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for RemoveLegacyLogSinkWriter callback")
	}
	select {
	case <-addCh:
		c.Fatal("unexpected AddLegacyLogSinkWriter call on Loki switch")
	default:
	}
}

func (s *workerSuite) TestManageLegacyLogSinkWriterOnLogSinkSwitch(c *tc.C) {
	fixture := newFixture(c, "")
	events := make(chan backendEvent, 20)

	addCh := make(chan struct{}, 1)
	removeCh := make(chan struct{}, 1)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() { removeCh <- struct{}{} },
		AddLegacyLogSinkWriter:    func() error { addCh <- struct{}{}; return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	// Drain initial callbacks.
	select {
	case <-addCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for initial AddLegacyLogSinkWriter callback")
	}
	drainCh(c, removeCh)

	// Switch to Loki first.
	fixture.agent.setLokiConfig("http://loki/loki/api/v1/push", "")
	fixture.configChanged.Set(true)

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "routed",
	})
	waitForEvents(c, events, backendEvent{
		backend: "logsink",
		kind:    "stop",
	}, backendEvent{
		backend: "loki",
		kind:    "start",
	})

	// Drain Loki switch callbacks.
	select {
	case <-removeCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for RemoveLegacyLogSinkWriter callback")
	}

	// Switch back to LogSink.
	fixture.agent.setLokiConfig("", "")
	fixture.configChanged.Set(true)

	sendLog(c, fixture.logs, &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "returned",
	})
	waitForEvents(c, events, backendEvent{
		backend: "loki",
		kind:    "stop",
	}, backendEvent{
		backend: "logsink",
		kind:    "start",
	})

	// Returning to LogSink should call AddLegacyLogSinkWriter.
	select {
	case <-addCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for AddLegacyLogSinkWriter callback")
	}
	select {
	case <-removeCh:
		c.Fatal("unexpected RemoveLegacyLogSinkWriter call on LogSink switch")
	default:
	}
}

func drainCh(c *tc.C, ch <-chan struct{}) {
	select {
	case <-ch:
	case <-c.Context().Done():
		c.Fatal("timed out draining channel")
	default:
	}
}

func (s *workerSuite) TestManageLegacyLogSinkWriterDrainOnly(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	events := make(chan backendEvent, 10)

	addCh := make(chan struct{}, 1)
	removeCh := make(chan struct{}, 1)

	w, err := NewWorker(WorkerConfig{
		Agent:                     fixture.agent,
		LogSource:                 fixture.logs,
		AgentConfigChanged:        fixture.configChanged,
		Logger:                    internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                     clock.WallClock,
		DrainOnly:                 true,
		ConvergeTimeout:           defaultConvergeTimeout,
		RestartDelay:              time.Millisecond * 10,
		NewBackend:                recordingBackendFunc(events, defaultBackendBufferSize),
		RemoveLegacyLogSinkWriter: func() { removeCh <- struct{}{} },
		AddLegacyLogSinkWriter:    func() error { addCh <- struct{}{}; return nil },
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	waitForEvents(c, events, backendEvent{
		backend: "drain-only",
		kind:    "start",
	})

	// In DrainOnly mode, the legacy writer should be present.
	select {
	case <-addCh:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for AddLegacyLogSinkWriter callback")
	}
	select {
	case <-removeCh:
		c.Fatal("unexpected RemoveLegacyLogSinkWriter call in DrainOnly mode")
	default:
	}
}

func sendLog(c *tc.C, logs logsender.LogRecordCh, record *logsender.LogRecord) {
	select {
	case logs <- record:
	case <-c.Context().Done():
		c.Fatalf("timed out sending log record %q", record.Message)
	}
}
