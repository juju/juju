// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"github.com/juju/errors"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
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

	cfg.ClockName = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.ClockName = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.PrometheusRegisterer = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.NewWorker = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg.NewStore = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:      "agent",
		ClockName:      "clock",
		DBAccessorName: "dbaccessor",

		Logger:               s.logger,
		PrometheusRegisterer: s.prometheusRegisterer,
		NewWorker: func(mc ManagerConfig) (worker.Worker, error) {
			return nil, nil
		},
		NewStore: func(coredatabase.DBGetter) lease.Store {
			return nil
		},
	}
}
