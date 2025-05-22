// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.ClockName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.DBAccessorName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.TraceName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.PrometheusRegisterer = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.NewStore = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.NewSecretaryFinder = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:      "agent",
		ClockName:      "clock",
		DBAccessorName: "dbaccessor",
		TraceName:      "trace",

		Logger:               s.logger,
		PrometheusRegisterer: s.prometheusRegisterer,
		NewWorker: func(mc ManagerConfig) (worker.Worker, error) {
			return nil, nil
		},
		NewStore: func(coredatabase.DBGetter, logger.Logger) lease.Store {
			return nil
		},
		NewSecretaryFinder: func(s string) lease.SecretaryFinder {
			return nil
		},
	}
}
