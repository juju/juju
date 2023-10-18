// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/objectstore"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewObjectStoreWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName: "agent",
		TraceName: "trace",
		Clock:     s.clock,
		Logger:    s.logger,
		NewObjectStoreWorker: func(objectstore.Type, string, objectstore.Config) (TrackedObjectStore, error) {
			return nil, nil
		},
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent": s.agent,
		"trace": stubTracerGetter{},
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent", "trace"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

type stubTracerGetter struct{}

// GetTracer returns a tracer for the given namespace.
func (stubTracerGetter) GetTracer(context.Context, coretrace.TracerNamespace) (coretrace.Tracer, error) {
	return coretrace.NoopTracer{}, nil
}
