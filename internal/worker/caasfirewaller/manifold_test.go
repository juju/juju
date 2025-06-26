// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	caasmocks "github.com/juju/juju/caas/mocks"
	"github.com/juju/juju/core/logger"
	applicationservice "github.com/juju/juju/domain/application/service"
	portservice "github.com/juju/juju/domain/port/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	getter   dependency.Getter

	broker         *caasmocks.MockBroker
	domainServices *mocks.MockModelDomainServices

	logger logger.Logger
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.ResetCalls()

	s.logger = loggertesting.WrapCheckLog(c)
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.broker = caasmocks.NewMockBroker(ctrl)

	s.domainServices = mocks.NewMockModelDomainServices(ctrl)
	s.domainServices.EXPECT().Port().Return(nil).AnyTimes()
	s.domainServices.EXPECT().Application().Return(nil).AnyTimes()

	s.getter = s.newGetter(nil)
	s.manifold = caasfirewaller.Manifold(s.validConfig())

	c.Cleanup(func() {
		s.broker = nil
		s.domainServices = nil
		s.getter = nil
	})

	return ctrl
}

func (s *manifoldSuite) validConfig() caasfirewaller.ManifoldConfig {
	return caasfirewaller.ManifoldConfig{
		BrokerName:         "broker",
		DomainServicesName: "domain-services",
		ControllerUUID:     coretesting.ControllerTag.Id(),
		ModelUUID:          coretesting.ModelTag.Id(),
		NewWorker:          s.newWorker,
		Logger:             s.logger,
	}
}

func (s *manifoldSuite) newWorker(config caasfirewaller.Config) (worker.Worker, error) {
	s.MethodCall(s, "NewWorker", config)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	w, err := worker.NewRunner(worker.RunnerParams{Name: "test"})
	if err != nil {
		return nil, err
	}
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, w) })
	return w, nil
}

func (s *manifoldSuite) newGetter(overlay map[string]interface{}) dependency.Getter {
	resources := map[string]interface{}{
		"broker":          s.broker,
		"domain-services": s.domainServices,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) TestMissingControllerUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.validConfig()
	config.ControllerUUID = ""
	s.checkConfigInvalid(c, config, "empty ControllerUUID not valid")
}

func (s *manifoldSuite) TestMissingModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.validConfig()
	config.ModelUUID = ""
	s.checkConfigInvalid(c, config, "empty ModelUUID not valid")
}

func (s *manifoldSuite) TestMissingBrokerName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.validConfig()
	config.BrokerName = ""
	s.checkConfigInvalid(c, config, "empty BrokerName not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.validConfig()
	config.NewWorker = nil
	s.checkConfigInvalid(c, config, "nil NewWorker not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.validConfig()
	config.Logger = nil
	s.checkConfigInvalid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) checkConfigInvalid(c *tc.C, config caasfirewaller.ManifoldConfig, expect string) {
	err := config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"broker", "domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(c.Context(), getter)
		c.Assert(errors.Cause(err), tc.Equals, dependency.ErrMissing)
	}
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewWorker")

	s.CheckCall(c, 0, "NewWorker", caasfirewaller.Config{
		ControllerUUID:     coretesting.ControllerTag.Id(),
		ModelUUID:          coretesting.ModelTag.Id(),
		Broker:             s.broker,
		Logger:             s.logger,
		PortService:        (*portservice.WatchableService)(nil),
		ApplicationService: (*applicationservice.WatchableService)(nil),
	})
}
