// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	caasmocks "github.com/juju/juju/caas/mocks"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	applicationservice "github.com/juju/juju/domain/application/service"
	portservice "github.com/juju/juju/domain/port/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
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

// TestValidateConfig tests the validation of [ManifoldConfig].
func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	c.Run("valid", func(c *testing.T) {
		config := s.validConfig()
		tc.Check(c, config.Validate(), tc.ErrorIsNil)
	})

	c.Run("empty broker name", func(c *testing.T) {
		config := s.validConfig()
		config.BrokerName = ""
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing new worker", func(c *testing.T) {
		config := s.validConfig()
		config.NewWorker = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing logger", func(c *testing.T) {
		config := s.validConfig()
		config.Logger = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

var expectedInputs = []string{"broker", "domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(s.manifold.Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestMissingInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	for _, input := range expectedInputs {
		c.Run("missing input "+input, func(c *testing.T) {
			getter := s.newGetter(map[string]interface{}{
				input: dependency.ErrMissing,
			})
			_, err := s.manifold.Start(c.Context(), getter)
			tc.Check(c, err, tc.ErrorIs, dependency.ErrMissing)
		})
	}
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)

	s.CheckCallNames(c, "NewWorker")

	s.CheckCall(c, 0, "NewWorker", caasfirewaller.Config{
		Broker:             s.broker,
		Logger:             s.logger,
		PortService:        (*portservice.WatchableService)(nil),
		ApplicationService: (*applicationservice.WatchableService)(nil),
	})
}
