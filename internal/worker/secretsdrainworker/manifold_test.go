// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/leadership"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujusecrets "github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
	"github.com/juju/juju/internal/worker/secretsdrainworker/mocks"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config secretsdrainworker.ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldSuite) validConfig(c *tc.C) secretsdrainworker.ManifoldConfig {
	return secretsdrainworker.ManifoldConfig{
		APICallerName:         "api-caller",
		LeadershipTrackerName: "leadership-tracker",
		Logger:                loggertesting.WrapCheckLog(c),
		NewWorker: func(config secretsdrainworker.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewSecretsDrainFacade: func(base.APICaller) secretsdrainworker.SecretsDrainFacade { return nil },
		NewBackendsClient: func(base.APICaller) (jujusecrets.BackendsClient, error) {
			return nil, nil
		},
	}
}

func (s *ManifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}
func (s *ManifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingNewFacade(c *tc.C) {
	s.config.NewSecretsDrainFacade = nil
	s.checkNotValid(c, "nil NewSecretsDrainFacade not valid")
}

func (s *ManifoldSuite) TestMissingNewBackendsClient(c *tc.C) {
	s.config.NewBackendsClient = nil
	s.checkNotValid(c, "nil NewBackendsClient not valid")
}

func (s *ManifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockSecretsDrainFacade(ctrl)
	s.config.NewSecretsDrainFacade = func(base.APICaller) secretsdrainworker.SecretsDrainFacade {
		return facade
	}

	backendClients := mocks.NewMockBackendsClient(ctrl)
	s.config.NewBackendsClient = func(base.APICaller) (jujusecrets.BackendsClient, error) {
		return backendClients, nil
	}

	called := false
	s.config.NewWorker = func(config secretsdrainworker.Config) (worker.Worker, error) {
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, tc.NotNil)
		mc.AddExpr(`_.Logger`, tc.NotNil)
		mc.AddExpr(`_.SecretsBackendGetter`, tc.NotNil)
		mc.AddExpr(`_.LeadershipTrackerFunc`, tc.NotNil)
		c.Check(config, mc, secretsdrainworker.Config{SecretsDrainFacade: facade})
		return nil, nil
	}
	manifold := secretsdrainworker.Manifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"api-caller":         struct{ base.APICaller }{&mockAPICaller{}},
		"leadership-tracker": struct{ leadership.TrackerWorker }{&mockLeadershipTracker{}},
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *ManifoldSuite) TestStartNoLeadershipTracker(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockSecretsDrainFacade(ctrl)
	s.config.NewSecretsDrainFacade = func(base.APICaller) secretsdrainworker.SecretsDrainFacade {
		return facade
	}
	s.config.LeadershipTrackerName = ""

	backendClients := mocks.NewMockBackendsClient(ctrl)
	s.config.NewBackendsClient = func(base.APICaller) (jujusecrets.BackendsClient, error) {
		return backendClients, nil
	}

	called := false
	s.config.NewWorker = func(config secretsdrainworker.Config) (worker.Worker, error) {
		called = true
		mc := tc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, tc.NotNil)
		mc.AddExpr(`_.Logger`, tc.NotNil)
		mc.AddExpr(`_.SecretsBackendGetter`, tc.NotNil)
		mc.AddExpr(`_.LeadershipTrackerFunc`, tc.NotNil)
		c.Check(config, mc, secretsdrainworker.Config{SecretsDrainFacade: facade})
		return nil, nil
	}
	manifold := secretsdrainworker.Manifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

type mockAPICaller struct {
	base.APICaller
}

func (*mockAPICaller) BestFacadeVersion(facade string) int {
	return 1
}

type mockLeadershipTracker struct {
	leadership.TrackerWorker
}
