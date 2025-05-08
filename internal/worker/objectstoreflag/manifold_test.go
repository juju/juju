// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreflag

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	tc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	objectstore "github.com/juju/juju/core/objectstore"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Check = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GeObjectStoreServicesFn = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ObjectStoreServicesName: "object-store-services",
		Check: func(p objectstore.Phase) bool {
			return true
		},
		GeObjectStoreServicesFn: func(getter dependency.Getter, name string) (ObjectStoreService, error) {
			return s.service, nil
		},
		NewWorker: func(ctx context.Context, c Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"object-store-services": &stubObjectStoreServicesGetter{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"object-store-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

// Note: This replicates the ability to get a controller domain services and
// a model domain services from the domain services getter.
type stubObjectStoreServicesGetter struct {
	services.ObjectStoreServices
	services.ObjectStoreServicesGetter
}

func (s *stubObjectStoreServicesGetter) ServicesForModel(model.UUID) services.ObjectStoreServices {
	return &stubObjectStoreServices{}
}

type stubObjectStoreServices struct {
	services.ObjectStoreServices
}

func (s *stubObjectStoreServices) ObjectStore() *objectstoreservice.WatchableService {
	return nil
}
