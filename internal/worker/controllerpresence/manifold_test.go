// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerpresence

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	controllerDomainServices *MockControllerDomainServices
	domainServices           *MockDomainServices
	modelService             *MockModelService
	statusService            *MockStatusService
	apiRemoteSubscriber      *MockAPIRemoteSubscriber
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold(c).Inputs, tc.DeepEquals, []string{"api-remote-caller", "domain-services"})
}

func (s *ManifoldSuite) TestValidate(c *tc.C) {
	err := s.newConfig(c).Validate()
	c.Assert(err, tc.IsNil)

	config := s.newConfig(c)
	config.APIRemoteCallerName = ""
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.DomainServicesName = ""
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.NewWorker = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.GetDomainServices = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.GetControllerDomainServices = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.Logger = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)

	config = s.newConfig(c)
	config.Clock = nil
	err = config.Validate()
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.controllerDomainServices.EXPECT().Model().Return(s.modelService)
	s.modelService.EXPECT().GetControllerModelUUID(gomock.Any()).Return(model.UUID("controller-uuid"), nil)
	s.domainServices.EXPECT().Status().Return(s.statusService)

	w, err := s.manifold(c).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) manifold(c *tc.C) dependency.Manifold {
	return Manifold(s.newConfig(c))
}

func (s *ManifoldSuite) newConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		APIRemoteCallerName: "api-remote-caller",
		DomainServicesName:  "domain-services",
		GetDomainServices: func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (DomainServices, error) {
			return s.domainServices, nil
		},
		GetControllerDomainServices: func(getter dependency.Getter, name string) (ControllerDomainServices, error) {
			return s.controllerDomainServices, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
		NewWorker: func(wc WorkerConfig) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
	}
}

func (s *ManifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"api-remote-caller": s.apiRemoteSubscriber,
	}
	return dependencytesting.StubGetter(resources)
}

func (s *ManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiRemoteSubscriber = NewMockAPIRemoteSubscriber(ctrl)
	s.domainServices = NewMockDomainServices(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.controllerDomainServices = NewMockControllerDomainServices(ctrl)

	c.Cleanup(func() {
		s.apiRemoteSubscriber = nil
		s.domainServices = nil
		s.modelService = nil
		s.statusService = nil
		s.controllerDomainServices = nil
	})

	return ctrl
}
