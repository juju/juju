// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoreguard

import (
	"testing"

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

func (s *manifoldSuite) TestInputs(c *tc.C) {
	config := s.config(c)
	c.Check(Manifold(config).Inputs, tc.SameContents, []string{
		"object-store-services",
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
		"fortress": guard,
	}
	getter := dependencytesting.StubGetter(resources)
	w, err := Manifold(config).Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) config(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		ObjectStoreServicesName: "object-store-services",
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
