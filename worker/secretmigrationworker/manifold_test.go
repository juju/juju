// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretmigrationworker_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/worker/secretmigrationworker"
	"github.com/juju/juju/worker/secretmigrationworker/mocks"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config secretmigrationworker.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldSuite) validConfig() secretmigrationworker.ManifoldConfig {
	return secretmigrationworker.ManifoldConfig{
		APICallerName: "api-caller",
		Logger:        loggo.GetLogger("test"),
		NewWorker: func(config secretmigrationworker.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewFacade: func(base.APICaller) secretmigrationworker.Facade { return nil },
		NewBackendsClient: func(jujusecrets.JujuAPIClient) (jujusecrets.BackendsClient, error) {
			return nil, nil
		},
	}
}

func (s *ManifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}
func (s *ManifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *ManifoldSuite) TestMissingNewBackendsClient(c *gc.C) {
	s.config.NewBackendsClient = nil
	s.checkNotValid(c, "nil NewBackendsClient not valid")
}

func (s *ManifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockFacade(ctrl)
	s.config.NewFacade = func(base.APICaller) secretmigrationworker.Facade {
		return facade
	}
	backendClients := mocks.NewMockBackendsClient(ctrl)
	s.config.NewBackendsClient = func(jujusecrets.JujuAPIClient) (jujusecrets.BackendsClient, error) {
		return backendClients, nil
	}

	called := false
	s.config.NewWorker = func(config secretmigrationworker.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Facade`, gc.NotNil)
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.SecretsBackendGetter`, gc.NotNil)
		c.Check(config, mc, secretmigrationworker.Config{Facade: facade})
		return nil, nil
	}
	manifold := secretmigrationworker.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]interface{}{
		"api-caller": struct{ base.APICaller }{&mockAPICaller{}},
	}))
	c.Assert(w, gc.IsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type mockAPICaller struct {
	base.APICaller
}

func (*mockAPICaller) BestFacadeVersion(facade string) int {
	return 1
}
