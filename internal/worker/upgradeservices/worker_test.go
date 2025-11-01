// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	changestream "github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	internalservices "github.com/juju/juju/internal/services"
)

// workerSuite provides a suite of tests for asserting the bahaviour of the
// worker offered by this package.
type workerSuite struct {
	baseSuite
}

// TestWorkerSuite runs the tests contained within [workerSuite].
func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Run("ValidConfig", func(t *testing.T) {
		cfg := s.getConfig()
		tc.Check(t, cfg.Validate(), tc.ErrorIsNil)
	})

	c.Run("DBGetter", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.DBGetter = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("Logger", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.Logger = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("NewUpgradeServices", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.NewUpgradeServices = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("NewUpgradeServicesGetter", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.NewUpgradeServices = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

// TestWorkerServicesGetter tests that the [servicesWorker] correctly returns
// [internalservices.UpgradeServicesGetter].
func (s *workerSuite) TestWorkerServicesGetter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBGetter:                 s.dbGetter,
		Logger:                   s.logger,
		NewUpgradeServicesGetter: NewUpgradeServicesGetter,
		NewUpgradeServices:       NewUpgradeServices,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*servicesWorker)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement servicesWorker"))

	factory := srvFact.ServicesGetter()
	c.Assert(factory, tc.NotNil)

	workertest.CleanKill(c, w)
}

// getConfig returns a filled out [Config] that can be used for validation
// testing.
func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter: s.dbGetter,
		Logger:   s.logger,
		NewUpgradeServices: func(
			changestream.WatchableDBGetter, logger.Logger,
		) internalservices.UpgradeServices {
			return nil
		},
		NewUpgradeServicesGetter: func(
			UpgradeServicesFn, changestream.WatchableDBGetter, logger.Logger,
		) internalservices.UpgradeServicesGetter {
			return nil
		},
	}
}
