// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	caasmocks "github.com/juju/juju/caas/mocks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
	coretesting "github.com/juju/juju/testing"
)

type manifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	getter   dependency.Getter

	apiCaller *mocks.MockAPICaller
	broker    *caasmocks.MockBroker
	client    *mocks.MockClient

	ctrl *gomock.Controller
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.ctrl = gomock.NewController(c)
	s.apiCaller = mocks.NewMockAPICaller(s.ctrl)
	s.broker = caasmocks.NewMockBroker(s.ctrl)
	s.client = mocks.NewMockClient(s.ctrl)

	s.getter = s.newGetter(nil)
	s.manifold = caasfirewaller.Manifold(s.validConfig(c))
}

func (s *manifoldSuite) validConfig(c *gc.C) caasfirewaller.ManifoldConfig {
	return caasfirewaller.ManifoldConfig{
		APICallerName:  "api-caller",
		BrokerName:     "broker",
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		NewClient:      s.newClient,
		NewWorker:      s.newWorker,
		Logger:         loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldSuite) newClient(apiCaller base.APICaller) caasfirewaller.Client {
	s.MethodCall(s, "NewClient", apiCaller)
	return s.client
}

func (s *manifoldSuite) newWorker(config caasfirewaller.Config) (worker.Worker, error) {
	s.MethodCall(s, "NewWorker", config)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *manifoldSuite) newGetter(overlay map[string]interface{}) dependency.Getter {
	resources := map[string]interface{}{
		"api-caller": s.apiCaller,
		"broker":     s.broker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) TestMissingControllerUUID(c *gc.C) {
	config := s.validConfig(c)
	config.ControllerUUID = ""
	s.checkConfigInvalid(c, config, "empty ControllerUUID not valid")
}

func (s *manifoldSuite) TestMissingModelUUID(c *gc.C) {
	config := s.validConfig(c)
	config.ModelUUID = ""
	s.checkConfigInvalid(c, config, "empty ModelUUID not valid")
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	config := s.validConfig(c)
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *gc.C) {
	config := s.validConfig(c)
	config.BrokerName = ""
	s.checkConfigInvalid(c, config, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *gc.C) {
	config := s.validConfig(c)
	config.NewWorker = nil
	s.checkConfigInvalid(c, config, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	config := s.validConfig(c)
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *gc.C, config caasfirewaller.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"api-caller", "broker"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewClient", "NewWorker")
	s.CheckCall(c, 0, "NewClient", s.apiCaller)

	s.CheckCall(c, 1, "NewWorker", caasfirewaller.Config{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		FirewallerAPI:  s.client,
		LifeGetter:     s.client,
		Broker:         s.broker,
		Logger:         loggertesting.WrapCheckLog(c),
	})
}
