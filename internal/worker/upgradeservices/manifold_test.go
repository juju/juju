// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeservices

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	internalservices "github.com/juju/juju/internal/services"
)

// manifoldSuite provides a suite of tests for asserting the bahaviour of the
// dependency manifold offered by this package.
type manifoldSuite struct {
	baseSuite
}

// TestManifoldSuite runs all of the tests contained within [manifoldSuite].
func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

// TestManifoldInputs ensures the inputs of this worker are thought about by
// future changes in this package.
//
// STOP: If this test has broken and new inputs have been added. It is a MUST
// that this input be checked that it will not cause a dead lock in the
// consuming manifold when a database upgrade is to be performed.
func (s *manifoldSuite) TestManifoldInputs(c *tc.C) {
	inputs := Manifold(s.getConfig()).Inputs
	c.Check(inputs, tc.SameContents, []string{"changestream"})
}

// TestManifoldStart ensures that the manifold starts the returned worker.
func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getter := map[string]any{
		"changestream": s.dbGetter,
	}

	manifold := Manifold(ManifoldConfig{
		ChangeStreamName:         "changestream",
		Logger:                   s.logger,
		NewWorker:                NewWorker,
		NewUpgradeServices:       NewUpgradeServices,
		NewUpgradeServicesGetter: NewUpgradeServicesGetter,
	})
	w, err := manifold.Start(c.Context(), dt.StubGetter(getter))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
}

// TestOutputUpgradeServicesGetter ensures that a
// [internalservices.UpgradeServicesGetter] can be retrieved from the manifold.
func (s *manifoldSuite) TestOutputUpgradeServicesGetter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBGetter:                 s.dbGetter,
		Logger:                   s.logger,
		NewUpgradeServices:       NewUpgradeServices,
		NewUpgradeServicesGetter: NewUpgradeServicesGetter,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory internalservices.UpgradeServicesGetter
	err = manifold.output(w, &factory)
	c.Check(err, tc.ErrorIsNil)
}

// TestOutputInvalid ensures that an error is returned when an invalid type
// is requested from the manifold output.
func (s *manifoldSuite) TestOutputInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBGetter:                 s.dbGetter,
		Logger:                   s.logger,
		NewUpgradeServices:       NewUpgradeServices,
		NewUpgradeServicesGetter: NewUpgradeServicesGetter,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var badType struct{}
	err = manifold.output(w, &badType)
	c.Check(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestValidateConfig asserts that [ManifoldConfig] validation fails and
// succeeds under the expected cases of values not being supplied.
func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Run("ValidConfig", func(t *testing.T) {
		cfg := s.getConfig()
		tc.Check(t, cfg.Validate(), tc.ErrorIsNil)
	})

	c.Run("ChangeStreamName", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.ChangeStreamName = ""
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
		cfg.NewUpgradeServicesGetter = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("NewWorker", func(t *testing.T) {
		cfg := s.getConfig()
		cfg.NewWorker = nil
		tc.Check(t, cfg.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

// getConfig returns a filled out [ManifoldConfig] that can be used for
// validation testing.
func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ChangeStreamName: "changestream",
		Logger:           s.logger,
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
		NewWorker: func(Config) (worker.Worker, error) {
			return nil, nil
		},
	}
}
