// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/fortress"
)

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

type manifoldSuite struct{}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	config := s.config(c)
	c.Check(config.Validate(), tc.ErrorIsNil)

	config = s.config(c)
	config.ObjectStoreServicesName = ""
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.config(c)
	config.ObjectStoreName = ""
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.config(c)
	config.FortressName = ""
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.config(c)
	config.GetDrainingService = nil
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.config(c)
	config.NewWorker = nil
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.config(c)
	config.Logger = nil
	c.Check(config.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	config := s.config(c)
	c.Check(Manifold(config).Inputs, tc.SameContents, []string{
		"object-store-services",
		"object-store",
		"fortress",
	})
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	guard := &recordingGuard{calls: make(chan string, 1)}
	service := &drainingService{
		watcher: watchertest.NewMockNotifyWatcher(make(chan struct{})),
		phases:  make(chan objectstore.Phase),
	}
	config := s.config(c)
	config.GetDrainingService = func(dependency.Getter, string) (DrainingService, error) {
		return service, nil
	}
	resources := map[string]any{
		"fortress":     guard,
		"object-store": &recordingFlusher{calls: make(chan struct{}, 1)},
	}
	getter := dependencytesting.StubGetter(resources)
	w, err := Manifold(config).Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestStartGetDrainingServiceError(c *tc.C) {
	config := s.config(c)
	config.GetDrainingService = func(dependency.Getter, string) (DrainingService, error) {
		return nil, errors.New("boom")
	}

	_, err := Manifold(config).Start(c.Context(), dependencytesting.StubGetter(nil))
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *manifoldSuite) TestStartMissingFortress(c *tc.C) {
	config := s.config(c)
	config.GetDrainingService = func(dependency.Getter, string) (DrainingService, error) {
		return &drainingService{}, nil
	}

	_, err := Manifold(config).Start(c.Context(), dependencytesting.StubGetter(map[string]any{
		"object-store": &recordingFlusher{calls: make(chan struct{}, 1)},
	}))
	c.Check(err, tc.Not(tc.ErrorIsNil))
}

func (s *manifoldSuite) TestStartMissingObjectStore(c *tc.C) {
	config := s.config(c)
	config.GetDrainingService = func(dependency.Getter, string) (DrainingService, error) {
		return &drainingService{}, nil
	}

	_, err := Manifold(config).Start(c.Context(), dependencytesting.StubGetter(map[string]any{
		"fortress": &recordingGuard{calls: make(chan string, 1)},
	}))
	c.Check(err, tc.Not(tc.ErrorIsNil))
}

func (s *manifoldSuite) config(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		ObjectStoreServicesName: "object-store-services",
		ObjectStoreName:         "object-store",
		FortressName:            "fortress",
		GetDrainingService: func(dependency.Getter, string) (DrainingService, error) {
			return nil, nil
		},
		NewWorker: func(config Config) (worker.Worker, error) {
			return NewWorker(config)
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
}

var _ fortress.Guard = (*recordingGuard)(nil)
