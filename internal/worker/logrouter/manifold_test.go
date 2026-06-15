// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logrouter

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	internallogger "github.com/juju/juju/internal/logger"
)

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	manifold := Manifold(ManifoldConfig{
		AgentName: "agent",
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"agent"})
}

func (s *manifoldSuite) TestStartReturnsGetterError(c *tc.C) {
	expectErr := stderrors.New("missing agent")
	manifold := Manifold(ManifoldConfig{
		AgentName: "agent",
	})

	w, err := manifold.Start(c.Context(), manifoldGetter{err: expectErr})
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, expectErr)
}

func (s *manifoldSuite) TestStartCreatesWorkerWithoutOpeningAPI(c *tc.C) {
	fixture := newFixture(c, "http://loki/loki/api/v1/push")
	var apiOpenCalled atomic.Bool
	manifold := Manifold(ManifoldConfig{
		AgentName:          "agent",
		LogSource:          fixture.logs,
		AgentConfigChanged: fixture.configChanged,
		Logger:             internallogger.GetLogger("juju.worker.logrouter.test"),
		Clock:              clock.WallClock,
		APIOpen: func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
			apiOpenCalled.Store(true)
			return nil, stderrors.New("api should not be opened during start")
		},
	})

	w, err := manifold.Start(c.Context(), manifoldGetter{agent: fixture.agent})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(apiOpenCalled.Load(), tc.IsFalse)
}

type manifoldGetter struct {
	agent coreagent.Agent
	err   error
}

func (g manifoldGetter) Get(_ string, out any) error {
	if g.err != nil {
		return g.err
	}
	switch out := out.(type) {
	case *coreagent.Agent:
		*out = g.agent
	default:
		return stderrors.New("unexpected dependency request")
	}
	return nil
}
