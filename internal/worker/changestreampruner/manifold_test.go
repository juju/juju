// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestreampruner

import (
	"testing"

	clock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/goleak"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBAccessor = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewModelPruner = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DBAccessor: "dbaccessor",
		Clock:      s.clock,
		Logger:     loggertesting.WrapCheckLog(c),
		NewWorker: func(WorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		NewModelPruner: func(
			db coredatabase.TxnRunner,
			namespaceWindow NamespaceWindow,
			clock clock.Clock,
			logger logger.Logger,
		) worker.Worker {
			return nil
		},
	}
}
