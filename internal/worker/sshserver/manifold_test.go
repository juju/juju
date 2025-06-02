// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	MockRegisterer *MockRegisterer
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.SSHJump)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *manifoldSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.MockRegisterer = NewMockRegisterer(ctrl)

	return ctrl
}

func (s *manifoldSuite) newManifoldConfig(modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:                 loggo.GetLogger("test"),
		APICallerName:          "api-caller",
		JWTParserName:          "jwt-parser",
		SSHTunnelerName:        "ssh-tunneler",
		PrometheusRegisterer:   s.MockRegisterer,
	}

	if modifier != nil {
		modifier(cfg)
	}

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	// Check config as expected.

	cfg := s.newManifoldConfig(nil)
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Missing NewServerWrapperWorker.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Empty APICallerName.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.APICallerName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Empty SSHTunnelerName.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.SSHTunnelerName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Empty PrometheusRegisterer.
	cfg = s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.PrometheusRegisterer = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	// Setup the manifold
	manifold := Manifold(*s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		}
	}))

	s.MockRegisterer.EXPECT().Register(gomock.Any()).Return(nil)
	s.MockRegisterer.EXPECT().Unregister(gomock.Any()).Return(true)

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, gc.DeepEquals, []string{
		"api-caller", "jwt-parser", "ssh-tunneler",
	})

	// Start the worker
	w, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller":   mockAPICaller{},
			"jwt-parser":   &jwtparser.Parser{},
			"ssh-tunneler": &sshtunneler.Tracker{},
		}),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)
	workertest.CleanKill(c, w)
}

type mockAPICaller struct {
	base.APICaller
}

func (a mockAPICaller) BestFacadeVersion(facade string) int {
	return 0
}

func (s *manifoldSuite) TestManifoldUninstall(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	// Unset feature flag
	os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	manifold := Manifold(*s.newManifoldConfig(func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		}
	}))
	// Start the worker
	_, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
			"jwt-parser": &MockJWTParser{},
		}),
	)
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)

}
