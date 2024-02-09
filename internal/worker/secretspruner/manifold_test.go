// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/worker/secretspruner"
	"github.com/juju/juju/internal/worker/secretspruner/mocks"
)

type manifoldSuite struct {
	testing.IsolationSuite
	config secretspruner.ManifoldConfig
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *manifoldSuite) validConfig() secretspruner.ManifoldConfig {
	return secretspruner.ManifoldConfig{
		APICallerName: "api-caller",
		Logger:        loggo.GetLogger("test"),
		NewWorker: func(config secretspruner.Config) (worker.Worker, error) {
			return nil, nil
		},
		NewUserSecretsFacade: func(base.APICaller) secretspruner.SecretsFacade { return nil },
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
	s.config.NewUserSecretsFacade = nil
	s.checkNotValid(c, "nil NewUserSecretsFacade not valid")
}

func (s *manifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facade := mocks.NewMockSecretsFacade(ctrl)
	s.config.NewUserSecretsFacade = func(base.APICaller) secretspruner.SecretsFacade {
		return facade
	}

	called := false
	s.config.NewWorker = func(config secretspruner.Config) (worker.Worker, error) {
		called = true
		mc := jc.NewMultiChecker()
		mc.AddExpr(`_.Logger`, gc.NotNil)
		c.Check(config, mc, secretspruner.Config{SecretsFacade: facade})
		return nil, nil
	}
	manifold := secretspruner.Manifold(s.config)
	w, err := manifold.Start(context.Background(), dt.StubGetter(map[string]interface{}{
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
