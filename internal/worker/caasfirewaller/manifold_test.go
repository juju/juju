// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"maps"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasfirewaller/mocks"
)

type manifoldSuite struct {
	broker         *mocks.MockExtCAASBroker
	domainServices *mocks.MockModelDomainServices
	getter         dependency.Getter
	worker         *mocks.MockWorker
}

var expectedInputs = []string{"broker", "domain-services"}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.broker = mocks.NewMockExtCAASBroker(ctrl)
	s.domainServices = mocks.NewMockModelDomainServices(ctrl)
	s.getter = s.newGetter(nil)
	s.worker = mocks.NewMockWorker(ctrl)

	//s.domainServices.EXPECT().Application()

	c.Cleanup(func() {
		s.broker = nil
		s.domainServices = nil
		s.getter = nil
	})

	return ctrl
}

// TestValidateConfig ensures that [ManifoldConfig] both passes and fails
// validation for various configurations.
func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	c.Run("valid", func(c *testing.T) {
		config := s.validConfig(c)
		tc.Check(c, config.Validate(), tc.ErrorIsNil)
	})

	c.Run("empty broker name", func(c *testing.T) {
		config := s.validConfig(c)
		config.BrokerName = ""
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing new worker", func(c *testing.T) {
		config := s.validConfig(c)
		config.NewFirewallWorker = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})

	c.Run("missing logger", func(c *testing.T) {
		config := s.validConfig(c)
		config.Logger = nil
		tc.Check(c, config.Validate(), tc.ErrorIs, coreerrors.NotValid)
	})
}

// TestInputExpectations confirms the manifold's dependency inputs matches
// this expectation.
func (s *manifoldSuite) TestInputExpectations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	manifold := caasfirewaller.Manifold(s.validConfig(c.T))
	c.Check(manifold.Inputs, tc.SameContents, expectedInputs)
}

// TestMissingInputs ensures that if the manifold is missing inputs that the
// correct errors are returned.
func (s *manifoldSuite) TestMissingInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	c.Run("domain-services", func(t *testing.T) {
		getter := s.newGetter(map[string]any{
			"domain-services": dependency.ErrMissing,
			"broker":          s.broker,
		})

		manifold := caasfirewaller.Manifold(s.validConfig(t))
		_, err := manifold.Start(c.Context(), getter)
		tc.Check(c, err, tc.ErrorIs, dependency.ErrMissing)
	})

	c.Run("broker", func(t *testing.T) {
		getter := s.newGetter(map[string]any{
			"broker":          dependency.ErrMissing,
			"domain-services": s.domainServices,
		})

		manifold := caasfirewaller.Manifold(s.validConfig(t))
		_, err := manifold.Start(c.Context(), getter)
		tc.Check(c, err, tc.ErrorIs, dependency.ErrMissing)
	})
}

// TestStart is a happy path test that ensures the manifold correctly collects
// all of the dependencies and starts the worker without error.
func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var workerStarted bool
	newFirewallerWorker := func(caasfirewaller.FirewallerConfig) (worker.Worker, error) {
		workerStarted = true
		return s.worker, nil
	}

	config := caasfirewaller.ManifoldConfig{
		BrokerName:           "broker",
		DomainServicesName:   "domain-services",
		NewAppFirewallWorker: s.newAppFirewallWorker,
		NewFirewallWorker:    newFirewallerWorker,
		Logger:               loggertesting.WrapCheckLog(c),
	}

	s.domainServices.EXPECT().Application()

	manifold := caasfirewaller.Manifold(config)
	_, err := manifold.Start(c.Context(), s.newGetter(nil))
	c.Check(err, tc.ErrorIsNil)
	c.Check(workerStarted, tc.IsTrue)
}

func (s *manifoldSuite) newAppFirewallWorker(
	coreapplication.UUID,
	caasfirewaller.AppFirewallerConfig,
) (worker.Worker, error) {
	return s.worker, nil
}

func (s *manifoldSuite) newFirewallerWorker(
	caasfirewaller.FirewallerConfig,
) (worker.Worker, error) {
	return s.worker, nil
}

func (s *manifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"broker":          s.broker,
		"domain-services": s.domainServices,
	}
	maps.Copy(resources, overlay)
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) validConfig(t *testing.T) caasfirewaller.ManifoldConfig {
	return caasfirewaller.ManifoldConfig{
		BrokerName:           "broker",
		DomainServicesName:   "domain-services",
		NewAppFirewallWorker: s.newAppFirewallWorker,
		NewFirewallWorker:    s.newFirewallerWorker,
		Logger:               loggertesting.WrapCheckLog(t),
	}
}
