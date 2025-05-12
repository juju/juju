// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.SSHJump)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func newManifoldConfig(l loggo.Logger, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:                 l,
		APICallerName:          "api-caller",
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	l := loggo.GetLogger("test")
	// Check config as expected.

	cfg := newManifoldConfig(l, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Entirely missing.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Empty APICallerName.
	cfg = newManifoldConfig(l, func(cfg *ManifoldConfig) {
		cfg.APICallerName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		APICallerName: "api-caller",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:          loggo.GetLogger("test"),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, gc.DeepEquals, []string{
		"api-caller",
	})

	// Start the worker
	w, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
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
	// Unset feature flag
	os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	manifold := Manifold(ManifoldConfig{
		APICallerName: "api-caller",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return workertest.NewDeadWorker(nil), nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:          loggo.GetLogger("test"),
	})
	// Start the worker
	_, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
		}),
	)
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)

}
