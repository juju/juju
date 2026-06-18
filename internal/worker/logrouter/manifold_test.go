// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	corehttp "github.com/juju/juju/core/http"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/loki"
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

func (s *manifoldSuite) validManifoldConfig(c *tc.C) ManifoldConfig {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	return ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		NewBackendFunc: func(base.APICaller, loki.HTTPClient, clock.Clock, prometheus.Registerer) BackendFunc {
			return recordingBackendFunc(make(chan backendEvent, 10), defaultBackendBufferSize)
		},
	}
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
