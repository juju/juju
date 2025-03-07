// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/database/app"
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
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.QueryLoggerName = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.Hub = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.LogDir = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.PrometheusRegisterer = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewApp = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewDBWorker = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewMetricsCollector = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewNodeManager = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:            "agent",
		QueryLoggerName:      "query-logger",
		Clock:                s.clock,
		Hub:                  s.hub,
		Logger:               s.logger,
		LogDir:               "log-dir",
		PrometheusRegisterer: s.prometheusRegisterer,
		NewApp: func(string, ...app.Option) (DBApp, error) {
			return s.dbApp, nil
		},
		NewDBWorker: func(context.Context, DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error) {
			return nil, nil
		},
		NewMetricsCollector: func() *Collector {
			return &Collector{}
		},
		NewNodeManager: func(agent.Config, Logger, coredatabase.SlowQueryLogger) NodeManager {
			return s.nodeManager
		},
	}
}
