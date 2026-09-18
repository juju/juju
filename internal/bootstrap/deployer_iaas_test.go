// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corebase "github.com/juju/juju/core/base"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/bootstrap"
)

type deployerIAASSuite struct {
	baseSuite
}

func TestDeployerIAASSuite(t *testing.T) {
	tc.Run(t, &deployerIAASSuite{})
}

func (s *deployerIAASSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.ApplicationService = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.HostBaseFn = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.Equals, corebase.MakeDefaultBase("ubuntu", "22.04"))
}

func (s *deployerIAASSuite) TestEnsureControllerApplicationAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := s.controllerCharmInfo()
	s.iaasApplicationService.EXPECT().CreateIAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", errors.Annotate(applicationerrors.ApplicationAlreadyExists, "controller"))
	s.expectControllerApplicationExposure()

	err := s.newDeployer(c).EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerIAASSuite) TestEnsureControllerApplicationCreationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := s.controllerCharmInfo()
	expectedErr := errors.New("cannot create controller application")
	s.iaasApplicationService.EXPECT().CreateIAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", expectedErr)

	err := s.newDeployer(c).EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *deployerIAASSuite) TestEnsureControllerApplicationRetriesExposure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := s.controllerCharmInfo()
	deployer := s.newDeployer(c)
	expectedErr := errors.New("cannot expose controller application")
	gomock.InOrder(
		s.iaasApplicationService.EXPECT().CreateIAASApplication(
			gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
			*info.Origin, gomock.Any(), gomock.Any(),
		).Return("", nil),
		s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), bootstrap.ControllerApplicationName).Return(false, nil),
		s.applicationService.EXPECT().MergeExposeSettings(gomock.Any(), bootstrap.ControllerApplicationName, nil).Return(expectedErr),
	)

	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)

	// The application was created, but bootstrap is not complete until the
	// retry successfully exposes it.
	s.iaasApplicationService.EXPECT().CreateIAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", applicationerrors.ApplicationAlreadyExists)
	s.expectControllerApplicationExposure()

	err = deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerIAASSuite) newDeployer(c *tc.C) *IAASDeployer {
	deployer, err := NewIAASDeployer(s.newConfig(c))
	c.Assert(err, tc.IsNil)
	return deployer
}

func (s *deployerIAASSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	return ctrl
}

func (s *deployerIAASSuite) newConfig(c *tc.C) IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		ApplicationService: s.iaasApplicationService,
		HostBaseFn: func() (corebase.Base, error) {
			return corebase.MakeDefaultBase("ubuntu", "22.04"), nil
		},
	}
}
