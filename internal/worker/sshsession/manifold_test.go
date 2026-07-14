// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"os"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/api/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/featureflag"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) newConfig(c *tc.C, modifier func(*ManifoldConfig)) ManifoldConfig {
	cfg := ManifoldConfig{
		AgentName:                "agent",
		APICallerName:            "api-caller",
		AuthenticationWorkerName: "ssh-authkeys-updater",
		Logger:                   loggertesting.WrapCheckLog(c),
		NewWorker:                func(WorkerConfig) (worker.Worker, error) { return nil, nil },
		NewFacadeClient:          func(base.APICaller) FacadeClient { return nil },
	}
	modifier(&cfg)
	return cfg
}

func (s *manifoldSuite) TestValidate(c *tc.C) {
	c.Check(s.newConfig(c, func(*ManifoldConfig) {}).Validate(), tc.ErrorIsNil)

	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.AgentName = "" }).Validate(), tc.ErrorIs, coreerrors.NotValid)
	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.APICallerName = "" }).Validate(), tc.ErrorIs, coreerrors.NotValid)
	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.AuthenticationWorkerName = "" }).Validate(), tc.ErrorIs, coreerrors.NotValid)
	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.Logger = nil }).Validate(), tc.ErrorIs, coreerrors.NotValid)
	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.NewWorker = nil }).Validate(), tc.ErrorIs, coreerrors.NotValid)
	c.Check(s.newConfig(c, func(cfg *ManifoldConfig) { cfg.NewFacadeClient = nil }).Validate(), tc.ErrorIs, coreerrors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	manifold := Manifold(s.newConfig(c, func(*ManifoldConfig) {}))
	c.Check(manifold.Inputs, tc.SameContents, []string{"agent", "api-caller", "ssh-authkeys-updater"})
}

func (s *manifoldSuite) TestStartUninstallsWhenFeatureDisabled(c *tc.C) {
	// Ensure the feature flag is off.
	_ = os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	manifold := Manifold(s.newConfig(c, func(*ManifoldConfig) {}))
	_, err := manifold.Start(c.Context(), dt.StubGetter(nil))
	c.Check(err, tc.ErrorIs, dependency.ErrUninstall)
}
