// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/worker/secretsrevoker"
)

type manifoldSuite struct {
	testing.IsolationSuite
	config secretsrevoker.ManifoldConfig
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *manifoldSuite) validConfig() secretsrevoker.ManifoldConfig {
	return secretsrevoker.ManifoldConfig{
		Clock:         testclock.NewDilatedWallClock(time.Millisecond),
		APICallerName: "api-caller",
		Logger:        loggo.GetLogger("test"),
		NewWorker: func(config secretsrevoker.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewSecretsFacade: func(base.APICaller) secretsrevoker.SecretsRevokerFacade { return nil },
	}
}

func (s *manifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewSecretsFacade = nil
	s.checkNotValid(c, "nil NewSecretsFacade not valid")
}

func (s *manifoldSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *manifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := NewMockSecretsRevokerFacade(ctrl)
	s.config.NewSecretsFacade = func(base.APICaller) secretsrevoker.SecretsRevokerFacade {
		return facade
	}

	called := false
	s.config.NewWorker = func(config secretsrevoker.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Logger`, gc.NotNil)
		mc.AddExpr(`_.Clock`, gc.NotNil)
		mc.AddExpr(`_.QuantiseTime`, gc.NotNil)
		c.Check(config, mc, secretsrevoker.Config{Facade: facade})
		return nil, nil
	}
	manifold := secretsrevoker.Manifold(s.config)
	w, err := manifold.Start(dt.StubContext(nil, map[string]any{
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
