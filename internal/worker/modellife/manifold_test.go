// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modellife

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	modelService *MockModelService
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestValidateConfig(c *gc.C) {
	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ModelUUID = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Result = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetModelService = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"domainservices"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.newManifold(c).Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	for _, input := range expectedInputs {
		getter := s.newGetter(c, map[string]any{
			input: dependency.ErrMissing,
		})
		_, err := s.newManifold(c).Start(context.Background(), getter)
		c.Assert(err, jc.ErrorIs, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w, err := s.newManifold(c).Start(context.Background(), s.newGetter(c, map[string]any{
		"domainservices": s.modelService,
	}))
	c.Assert(err, jc.ErrorIsNil)

	workertest.CheckAlive(c, w)

	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) newManifold(c *gc.C) dependency.Manifold {
	manifold := Manifold(s.getConfig())
	return manifold
}

func (s *ManifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domainservices",
		ModelUUID:          "model-uuid",
		Result:             life.IsAlive,
		NewWorker: func(ctx context.Context, c Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		GetModelService: func(d dependency.Getter, name string) (ModelService, error) {
			var modelService ModelService
			if err := d.Get(name, &modelService); err != nil {
				return nil, err
			}
			return modelService, nil
		},
	}
}

func (s *ManifoldSuite) newGetter(c *gc.C, overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"domainservices": s.modelService,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *ManifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelService = NewMockModelService(ctrl)

	return ctrl
}
