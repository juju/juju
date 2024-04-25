// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBAccessor = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.FileNotifyWatcher = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewMetricsCollector = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWatchableDB = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		AgentName:            "agent",
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
