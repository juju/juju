// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	stderrors "errors"
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
		Agent:              fixture.agent,
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		ConvergeTimeout:    defaultConvergeTimeout,
		NewLogSinkBackend:  recordingBackendFactory("logsink", events, defaultBackendBufferSize),
		NewLokiBackend:     recordingBackendFactory("loki", events, defaultBackendBufferSize),
		NewDrainBackend:    recordingBackendFactory("drain-only", events, defaultBackendBufferSize),
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
		Agent:              fixture.agent,
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		ConvergeTimeout:    defaultConvergeTimeout,
		NewLogSinkBackend:  recordingBackendFactory("logsink", events, defaultBackendBufferSize),
		NewLokiBackend:     recordingBackendFactory("loki", events, defaultBackendBufferSize),
		NewDrainBackend:    recordingBackendFactory("drain-only", events, defaultBackendBufferSize),
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

	fixture.logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "routed",
	}
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
		Agent:              fixture.agent,
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		DrainOnly:          true,
		ConvergeTimeout:    defaultConvergeTimeout,
		NewLogSinkBackend:  recordingBackendFactory("logsink", events, defaultBackendBufferSize),
		NewLokiBackend:     recordingBackendFactory("loki", events, defaultBackendBufferSize),
		NewDrainBackend:    recordingBackendFactory("drain-only", events, defaultBackendBufferSize),
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
		Agent:              fixture.agent,
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		ConvergeTimeout:    defaultConvergeTimeout,
		NewLogSinkBackend:  failingBackendFactory("logsink", events, 1),
		NewLokiBackend:     recordingBackendFactory("loki", events, defaultBackendBufferSize),
		NewDrainBackend:    recordingBackendFactory("drain-only", events, defaultBackendBufferSize),
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

	fixture.logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "buffered",
	}
	fixture.logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "fallback",
	}

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
		Agent:              fixture.agent,
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		ConvergeTimeout:    time.Millisecond * 10,
		NewLogSinkBackend:  errorBackendFactory("logsink", events),
		NewLokiBackend:     recordingBackendFactory("loki", events, defaultBackendBufferSize),
		NewDrainBackend:    recordingBackendFactory("drain-only", events, defaultBackendBufferSize),
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

	fixture.logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "buffered",
	}
	fixture.logs <- &logsender.LogRecord{
		Time:    time.Now(),
		Module:  "test",
		Level:   loggo.INFO,
		Message: "fallback",
	}

	c.Check(waitForRecord(c, events, "drain-only", "fallback"), tc.DeepEquals, backendEvent{
		backend: "drain-only",
		kind:    "record",
		message: "fallback",
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
	cfg.SetLokiConfig(lokiEndpoint, "")
	return fixture{
		agent: &testAgent{
			cfg: cfg,
		},
		logs:          make(logsender.LogRecordCh),
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
	a.cfg.SetLokiConfig(endpoint, caCert)
}

type backendEvent struct {
	backend string
	kind    string
	message string
}

func recordingBackendFactory(name string, events chan<- backendEvent, backendBufferSize int) BackendFactory {
	return func(ConfigSnapshot) (Backend, error) {
		w := &recordingBackend{
			name:    name,
			records: make(logsender.LogRecordCh, backendBufferSize),
			events:  events,
		}
		events <- backendEvent{backend: name, kind: "start"}
		w.tomb.Go(w.loop)
		return w, nil
	}
}

func failingBackendFactory(name string, events chan<- backendEvent, backendBufferSize int) BackendFactory {
	return func(ConfigSnapshot) (Backend, error) {
		w := &failingBackend{
			records: make(logsender.LogRecordCh, backendBufferSize),
		}
		events <- backendEvent{backend: name, kind: "start"}
		w.tomb.Go(func() error {
			return stderrors.New("backend failed")
		})
		return w, nil
	}
}

func errorBackendFactory(name string, events chan<- backendEvent) BackendFactory {
	return func(ConfigSnapshot) (Backend, error) {
		events <- backendEvent{backend: name, kind: "start"}
		return nil, stderrors.New("backend start failed")
	}
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
