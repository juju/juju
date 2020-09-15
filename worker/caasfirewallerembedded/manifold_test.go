// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	caasmocks "github.com/juju/juju/caas/mocks"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasfirewallerembedded"
	"github.com/juju/juju/worker/caasfirewallerembedded/mocks"
)

type manifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	context  dependency.Context

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

	s.context = s.newContext(nil)
	s.manifold = caasfirewallerembedded.Manifold(s.validConfig())
}

func (s *manifoldSuite) validConfig() caasfirewallerembedded.ManifoldConfig {
	return caasfirewallerembedded.ManifoldConfig{
		APICallerName:  "api-caller",
		BrokerName:     "broker",
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		NewClient:      s.newClient,
		NewWorker:      s.newWorker,
		Logger:         loggo.GetLogger("test"),
	}
}

func (s *manifoldSuite) newClient(apiCaller base.APICaller) caasfirewallerembedded.Client {
	s.MethodCall(s, "NewClient", apiCaller)
	return s.client
}

func (s *manifoldSuite) newWorker(config caasfirewallerembedded.Config) (worker.Worker, error) {
	s.MethodCall(s, "NewWorker", config)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *manifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"api-caller": s.apiCaller,
		"broker":     s.broker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *manifoldSuite) TestMissingControllerUUID(c *gc.C) {
	config := s.validConfig()
	config.ControllerUUID = ""
	s.checkConfigInvalid(c, config, "empty ControllerUUID not valid")
}

func (s *manifoldSuite) TestMissingModelUUID(c *gc.C) {
	config := s.validConfig()
	config.ModelUUID = ""
	s.checkConfigInvalid(c, config, "empty ModelUUID not valid")
}

func (s *manifoldSuite) TestMissingAPICallerName(c *gc.C) {
	config := s.validConfig()
	config.APICallerName = ""
	s.checkConfigInvalid(c, config, "empty APICallerName not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *gc.C) {
	config := s.validConfig()
	config.BrokerName = ""
	s.checkConfigInvalid(c, config, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *gc.C) {
	config := s.validConfig()
	config.NewWorker = nil
	s.checkConfigInvalid(c, config, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *gc.C) {
	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *gc.C, config caasfirewallerembedded.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

var expectedInputs = []string{"api-caller", "broker"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewClient", "NewWorker")
	s.CheckCall(c, 0, "NewClient", s.apiCaller)

	s.CheckCall(c, 1, "NewWorker", caasfirewallerembedded.Config{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		FirewallerAPI:  s.client,
		LifeGetter:     s.client,
		Broker:         s.broker,
		Logger:         loggo.GetLogger("test"),
	})
}
