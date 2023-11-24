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

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	internalobjectstore "github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state"
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

	cfg.StateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.TraceName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.ServiceFactoryName = ""
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
		AgentName:          "agent",
		StateName:          "state",
		TraceName:          "trace",
		ServiceFactoryName: "service-factory",
		Clock:              s.clock,
		Logger:             s.logger,
		NewObjectStoreWorker: func(context.Context, objectstore.BackendType, string, ...internalobjectstore.Option) (internalobjectstore.TrackedObjectStore, error) {
			return nil, nil
		},
		GetObjectStoreType: func(ControllerConfigService) (objectstore.BackendType, error) {
			return objectstore.StateBackend, nil
		},
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent":           s.agent,
		"trace":           &stubTracerGetter{},
		"state":           s.stateTracker,
		"service-factory": &stubServiceFactory{},
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent", "state", "trace", "service-factory"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStateTracker()

	w, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectStateTracker() {
	s.stateTracker.EXPECT().Use().Return(&state.StatePool{}, &state.State{}, nil)
	s.stateTracker.EXPECT().Done()
}

type stubTracerGetter struct{}

func (s *stubTracerGetter) GetTracer(ctx context.Context, namespace trace.TracerNamespace) (trace.Tracer, error) {
	return trace.NoopTracer{}, nil
}

type stubServiceFactory struct {
	servicefactory.ControllerServiceFactory
}

func (s *stubServiceFactory) ControllerConfig() *controllerconfigservice.Service {
	return nil
}
