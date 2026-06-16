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

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	corehttp "github.com/juju/juju/core/http"
	internallogger "github.com/juju/juju/internal/logger"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	manifold := Manifold(ManifoldConfig{
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

func (s *manifoldSuite) TestStartCreatesWorkerWithoutOpeningAPI(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	var apiOpenCalled atomic.Bool
	cfg := s.validManifoldConfig(c)
	cfg.NewAPIOpen = func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		apiOpenCalled.Store(true)
		return nil, stderrors.New("api should not be opened during start")
	}
	manifold := Manifold(cfg)

	w, err := manifold.Start(c.Context(), manifoldGetter{agent: fixture.agent})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(apiOpenCalled.Load(), tc.IsFalse)
}

func (s *manifoldSuite) validManifoldConfig(c *tc.C) ManifoldConfig {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	return ManifoldConfig{
		AgentName:          "agent",
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		HTTPClientName:     "http-client",
		NewAPIOpen: func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
			return nil, nil
		},
		NewBackendFunc: func(coreagent.Agent, func(context.Context, *api.Info, api.DialOpts) (api.Connection, error), corehttp.HTTPClientGetter, clock.Clock) BackendFunc {
			return recordingBackendFunc(make(chan backendEvent, 10), defaultBackendBufferSize)
		},
	}
}

type manifoldGetter struct {
	agent            coreagent.Agent
	httpClientGetter corehttp.HTTPClientGetter
	called           *atomic.Bool
	err              error
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
	case *corehttp.HTTPClientGetter:
		httpClientGetter := g.httpClientGetter
		if httpClientGetter == nil {
			httpClientGetter = stubHTTPClientGetter{client: http.DefaultClient}
		}
		*out = httpClientGetter
	default:
		return stderrors.New("unexpected dependency request")
	}
	return nil
}

type stubHTTPClientGetter struct {
	client corehttp.HTTPClient
}

func (g stubHTTPClientGetter) GetHTTPClient(context.Context, corehttp.Purpose) (corehttp.HTTPClient, error) {
	return g.client, nil
}
