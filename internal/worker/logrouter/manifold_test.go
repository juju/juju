// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	stderrors "errors"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/loki"
	"github.com/juju/juju/internal/worker/logsender"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	manifold := Manifold(ManifoldConfig{
		AgentName:      "agent",
		APICallerName:  "api-caller",
		HTTPClientName: "http-client",
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"agent", "api-caller", "http-client"})
}

func (s *manifoldSuite) TestControllerInputs(c *tc.C) {
	manifold := ControllerManifold(ControllerManifoldConfig{
		AgentName:      "agent",
		HTTPClientName: "http-client",
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"agent", "http-client"})
}

func (s *manifoldSuite) TestValidateAcceptsValidConfig(c *tc.C) {
	c.Check(s.validManifoldConfig(c).Validate(), tc.ErrorIsNil)
}

func (s *manifoldSuite) TestStartReturnsGetterError(c *tc.C) {
	expectErr := stderrors.New("missing agent")
	manifold := Manifold(s.validManifoldConfig(c))

	w, err := manifold.Start(c.Context(), manifoldGetter{err: expectErr})
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, expectErr)
}

func (s *manifoldSuite) TestStartValidatesBeforeGetter(c *tc.C) {
	var getterCalled atomic.Bool
	manifold := Manifold(ManifoldConfig{})

	w, err := manifold.Start(c.Context(), manifoldGetter{
		called: &getterCalled,
		err:    stderrors.New("getter should not be called"),
	})
	c.Check(w, tc.IsNil)
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `empty AgentName not valid`)
	c.Check(getterCalled.Load(), tc.IsFalse)
}

func (s *manifoldSuite) TestStartCreatesWorkerWithoutUsingAPICaller(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	cfg := s.validManifoldConfig(c)
	manifold := Manifold(cfg)

	w, err := manifold.Start(c.Context(), manifoldGetter{
		agent:     fixture.agent,
		apiCaller: stubAPICaller{},
		http:      stubHTTPClientGetter{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestControllerStartCreatesWorkerWithoutAPICaller(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	cfg := s.validControllerManifoldConfig(c)
	manifold := ControllerManifold(cfg)

	w, err := manifold.Start(c.Context(), manifoldGetter{
		agent: fixture.agent,
		http:  stubHTTPClientGetter{},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestNewBackendUpdatesLokiCACert(c *tc.C) {
	client := &recordingCACertUpdaterClient{}
	backendFunc := NewBackend(stubAPICaller{}, client, clock.WallClock, prometheus.NewRegistry())

	backend, err := backendFunc(BackendTypeLoki, ConfigSnapshot{
		Mode:          BackendTypeLoki,
		Endpoint:      "http://loki/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, backend)

	c.Check(client.caCert.Load(), tc.Equals, "ca-cert")
	c.Check(client.insecureSkipVerify.Load(), tc.IsFalse)
}

func (s *manifoldSuite) TestNewBackendReturnsCACertUpdateError(c *tc.C) {
	expectErr := stderrors.New("boom")
	client := &recordingCACertUpdaterClient{err: expectErr}
	backendFunc := NewBackend(stubAPICaller{}, client, clock.WallClock, prometheus.NewRegistry())

	backend, err := backendFunc(BackendTypeLoki, ConfigSnapshot{
		Mode:          BackendTypeLoki,
		Endpoint:      "http://loki/loki/api/v1/push",
		CACertificate: "ca-cert",
	})
	c.Check(backend, tc.IsNil)
	c.Assert(err, tc.ErrorIs, expectErr)
}

func (s *manifoldSuite) TestNewControllerBackendUsesLocalSinkForLogSinkMode(c *tc.C) {
	sink := &recordingLogSink{done: make(chan struct{}, 1)}
	backendFunc := NewControllerBackend(
		sink,
		stubHTTPClient{},
		clock.WallClock,
		prometheus.NewRegistry(),
	)

	backend, err := backendFunc(BackendTypeLogSink, ConfigSnapshot{
		Mode: BackendTypeLogSink,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, backend)

	backend.LogRecords() <- &logsender.LogRecord{
		Message:   "hello",
		ModelUUID: "model-uuid",
		Entity:    "unit-foo-0",
	}

	select {
	case <-sink.done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for local sink write")
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	c.Check(sink.records, tc.DeepEquals, []corelogger.LogRecord{{
		Message:   "hello",
		ModelUUID: "model-uuid",
		Entity:    "unit-foo-0",
	}})
}

func (s *manifoldSuite) TestNewControllerBackendUsesDrainBackendForDrainMode(c *tc.C) {
	sink := &recordingLogSink{done: make(chan struct{}, 1)}
	backendFunc := NewControllerBackend(
		sink,
		stubHTTPClient{},
		clock.WallClock,
		prometheus.NewRegistry(),
	)

	backend, err := backendFunc(BackendTypeDrain, ConfigSnapshot{
		Mode: BackendTypeDrain,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, backend)

	backend.LogRecords() <- &logsender.LogRecord{
		Message:   "drained",
		ModelUUID: "model-uuid",
		Entity:    "unit-foo-0",
	}

	for {
		report := backend.Report(c.Context())
		c.Assert(report["name"], tc.Equals, "drain-backend")
		if report["bufferedRecords"] == 0 {
			break
		}
		select {
		case <-c.Context().Done():
			c.Fatal("timed out waiting for drain backend to consume record")
		default:
		}
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	c.Check(sink.records, tc.HasLen, 0)
}

func (s *manifoldSuite) TestNewControllerBackendUsesLokiBackendForLokiMode(c *tc.C) {
	sink := &recordingLogSink{done: make(chan struct{}, 1)}
	backendFunc := NewControllerBackend(
		sink,
		stubHTTPClient{},
		clock.WallClock,
		prometheus.NewRegistry(),
	)

	backend, err := backendFunc(BackendTypeLoki, ConfigSnapshot{
		Mode:      BackendTypeLoki,
		Endpoint:  "http://loki/loki/api/v1/push",
		ModelUUID: "model-uuid",
		AgentID:   "machine-0",
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, backend)

	report := backend.Report(c.Context())
	c.Check(report["name"], tc.Equals, "loki-backend")

	sink.mu.Lock()
	defer sink.mu.Unlock()
	c.Check(sink.records, tc.HasLen, 0)
}

func (s *manifoldSuite) validManifoldConfig(c *tc.C) ManifoldConfig {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	return ManifoldConfig{
		AgentName:            "agent",
		APICallerName:        "api-caller",
		HTTPClientName:       "http-client",
		LogSource:            fixture.logs,
		AgentConfigChanged:   fixture.configChanged,
		Logger:               internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                clock.WallClock,
		PrometheusRegisterer: prometheus.NewRegistry(),
		NewBackendFunc: func(base.APICaller, loki.HTTPClient, clock.Clock, prometheus.Registerer) BackendFunc {
			return recordingBackendFunc(make(chan backendEvent, 10), defaultBackendBufferSize)
		},
		RemoveLegacyLogSinkWriter: func() {},
		AddLegacyLogSinkWriter:    func() error { return nil },
	}
}

func (s *manifoldSuite) validControllerManifoldConfig(c *tc.C) ControllerManifoldConfig {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	return ControllerManifoldConfig{
		AgentName:            "agent",
		HTTPClientName:       "http-client",
		AgentConfigChanged:   fixture.configChanged,
		Logger:               internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:                clock.WallClock,
		PrometheusRegisterer: prometheus.NewRegistry(),
		LocalLogSink:         &recordingLogSink{},
		NewBackendFunc:       NewControllerBackend,
	}
}

func (s *manifoldSuite) TestValidateRejectsNilRemoveLegacyLogSinkWriter(c *tc.C) {
	cfg := s.validManifoldConfig(c)
	cfg.RemoveLegacyLogSinkWriter = nil
	err := cfg.Validate()
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil RemoveLegacyLogSinkWriter not valid`)
}

func (s *manifoldSuite) TestValidateRejectsNilAddLegacyLogSinkWriter(c *tc.C) {
	cfg := s.validManifoldConfig(c)
	cfg.AddLegacyLogSinkWriter = nil
	err := cfg.Validate()
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil AddLegacyLogSinkWriter not valid`)
}

type manifoldGetter struct {
	agent     coreagent.Agent
	apiCaller base.APICaller
	http      corehttp.HTTPClientGetter
	called    *atomic.Bool
	err       error
}

func (g manifoldGetter) Get(_ string, out any) error {
	if g.called != nil {
		g.called.Store(true)
	}
	if g.err != nil {
		return g.err
	}
	switch out := out.(type) {
	case *coreagent.Agent:
		*out = g.agent
	case *base.APICaller:
		*out = g.apiCaller
	case *corehttp.HTTPClientGetter:
		if g.http == nil {
			g.http = stubHTTPClientGetter{}
		}
		*out = g.http
	default:
		return stderrors.New("unexpected dependency request")
	}
	return nil
}

type stubAPICaller struct {
	base.APICaller
}

type stubHTTPClientGetter struct{}

func (stubHTTPClientGetter) GetHTTPClient(context.Context, corehttp.Purpose) (corehttp.HTTPClient, error) {
	return stubHTTPClient{}, nil
}

type stubHTTPClient struct{}

func (stubHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, nil
}

type recordingLogSink struct {
	mu      sync.Mutex
	records []corelogger.LogRecord
	done    chan struct{}
}

func (r *recordingLogSink) Log(records []corelogger.LogRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done == nil {
		r.done = make(chan struct{}, 1)
	}
	r.records = append(r.records, slices.Clone(records)...)
	select {
	case r.done <- struct{}{}:
	default:
	}
	return nil
}

func (r *recordingLogSink) WatchRefresh() <-chan struct{} {
	return corelogger.NoRefresh()
}

type recordingCACertUpdaterClient struct {
	caCert             atomic.Value
	insecureSkipVerify atomic.Bool
	err                error
}

func (c *recordingCACertUpdaterClient) Do(*http.Request) (*http.Response, error) {
	return nil, nil
}

func (c *recordingCACertUpdaterClient) ReplaceCACert(caCert string, insecureSkipVerify bool) error {
	if c.err != nil {
		return c.err
	}
	c.caCert.Store(caCert)
	c.insecureSkipVerify.Store(insecureSkipVerify)
	return nil
}
