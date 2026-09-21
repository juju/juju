// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/status"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment/charm"
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

func (s *deployerIAASSuite) TestEnsureControllerApplication(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Creating a new controller application must also expose it before
	// bootstrap can complete.

	now := clock.WallClock.Now()
	mockClock := NewMockClock(ctrl)
	mockClock.EXPECT().Now().Return(now)

	cfg := s.newConfig(c)
	cfg.Clock = mockClock

	curl := "ch:juju-controller-0"

	// The application is called "controller" and the charm is called
	// "juju-controller". Do not change this, or the controller charm won't
	// come back up.

	s.iaasApplicationService.EXPECT().CreateIAASApplication(
		gomock.Any(),
		bootstrap.ControllerApplicationName,
		s.charm,
		corecharm.Origin{
			Source:   "charm-hub",
			Type:     "charm",
			Channel:  &charm.Channel{},
			Revision: new(1),
			Hash:     "sha-256",
			Platform: corecharm.Platform{
				Architecture: "arm64",
				OS:           "ubuntu",
				Channel:      "22.04",
			},
		},
		applicationservice.AddApplicationArgs{
			ReferenceName: bootstrap.ControllerCharmName,
			DownloadInfo: &applicationcharm.DownloadInfo{
				CharmhubIdentifier: "abcd",
				Provenance:         applicationcharm.ProvenanceBootstrap,
				DownloadURL:        "https://inferi.com",
				DownloadSize:       42,
			},
			CharmStoragePath:     "path",
			CharmObjectStoreUUID: "1234",
			ApplicationConfig: charm.Config{
				"is-juju":               true,
				"identity-provider-url": "https://inferi.com",
				"controller-url":        "wss://obscura.com:1234/api",
			},
			ApplicationSettings: domainapplication.ApplicationSettings{
				Trust: true,
			},
			ApplicationStatus: &status.StatusInfo{
				Status: status.Unset,
				Since:  new(now),
			},
			IsController: true,
		},
		applicationservice.AddIAASUnitArg{
			Nonce: new(agent.BootstrapNonce),
		},
	).Return("", nil)

	s.expectControllerApplicationExposure()

	deployer, err := NewIAASDeployer(cfg)
	c.Assert(err, tc.ErrorIsNil)

	origin := corecharm.Origin{
		Source:   corecharm.CharmHub,
		Type:     "charm",
		Channel:  &charm.Channel{},
		Revision: new(1),
		Hash:     "sha-256",
		Platform: corecharm.Platform{
			Architecture: "arm64",
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}
	err = deployer.EnsureControllerApplication(c.Context(), DeployCharmInfo{
		URL:    charm.MustParseURL(curl),
		Charm:  s.charm,
		Origin: &origin,
		DownloadInfo: &corecharm.DownloadInfo{
			CharmhubIdentifier: "abcd",
			DownloadURL:        "https://inferi.com",
			DownloadSize:       42,
		},
		ArchivePath:     "path",
		ObjectStoreUUID: "1234",
	})
	c.Assert(err, tc.ErrorIsNil)
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
