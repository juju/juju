// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/model"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)

	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.FortressName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GeObjectStoreServicesFn = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		FortressName:            "fortress",
		ObjectStoreServicesName: "object-store-services",
		GeObjectStoreServicesFn: func(getter dependency.Getter, name string) (ObjectStoreService, error) {
			return s.service, nil
		},
		NewWorker: func(ctx context.Context, c Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"fortress":              s.guard,
		"object-store-services": &stubObjectStoreServicesGetter{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"fortress", "object-store-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
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
