// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/mock/gomock"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ControllerID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBAccessor = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.FileNotifyWatcher = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewMetricsCollector = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWatchableDB = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestManifoldInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	inputs := Manifold(cfg).Inputs
	c.Check(inputs, tc.SameContents, []string{
		cfg.DBAccessor,
		cfg.FileNotifyWatcher,
	})
	// The agent manifold is no longer a direct input.
	for _, input := range inputs {
		c.Check(input, tc.Not(tc.Equals), "agent")
	}
}

func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectClock()
	s.prometheusRegisterer.EXPECT().Register(gomock.Any()).Return(nil)
	s.prometheusRegisterer.EXPECT().Unregister(gomock.Any()).Return(true)

	cfg := s.getConfig(c)
	// The getter only exposes the two direct inputs; any request for
	// the "agent" manifold would result in a missing-resource error,
	// which would bubble up and fail the assertion below.
	getter := dependencytesting.StubGetter(map[string]any{
		cfg.DBAccessor:        s.dbGetter,
		cfg.FileNotifyWatcher: s.fileNotifyWatcher,
	})

	w, err := Manifold(cfg).Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		ControllerID:         "0",
		DBAccessor:           "dbaccessor",
		FileNotifyWatcher:    "filenotifywatcher",
		Clock:                s.clock,
		Logger:               loggertesting.WrapCheckLog(c),
		NewMetricsCollector:  NewMetricsCollector,
		PrometheusRegisterer: s.prometheusRegisterer,
		NewWatchableDB: func(string, coredatabase.TxnRunner, FileNotifier, clock.Clock, NamespaceMetrics, logger.Logger) (WatchableDBWorker, error) {
			return nil, nil
		},
	}
}
