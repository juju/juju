// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	network "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/uuid"
)

type deployerCAASSuite struct {
	baseSuite
	clock          *MockClock
	serviceManager *MockServiceManager
}

func TestDeployerCAASSuite(t *testing.T) {
	tc.Run(t, &deployerCAASSuite{})
}

func (s *deployerCAASSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.ServiceManager = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerCAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.DeepEquals, version.DefaultSupportedLTSBase())
}

func (s *deployerCAASSuite) TestEnsureControllerApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	s.clock.EXPECT().Now().Return(now).AnyTimes()

	cfg := s.newConfig(c)
	cfg.Clock = s.clock
	cfg.Constraints = constraints.Value{Arch: new("arm64"), Mem: new(uint64(4096))}

	curl := "ch:juju-controller-0"
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

	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(),
		bootstrap.ControllerApplicationName,
		s.charm,
		origin,
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
			Constraints:  constraints.Value{Arch: new("arm64"), Mem: new(uint64(4096))},
			IsController: true,
		},
		applicationservice.AddUnitArg{},
	)
	s.expectControllerApplicationCompletion(cfg, nil)

	deployer := s.newDeployerWithConfig(c, cfg)

	downloadInfo := &corecharm.DownloadInfo{
		CharmhubIdentifier: "abcd",
		DownloadURL:        "https://inferi.com",
		DownloadSize:       42,
	}
	err := deployer.EnsureControllerApplication(c.Context(), DeployCharmInfo{
		URL:             charm.MustParseURL(curl),
		Charm:           s.charm,
		Origin:          &origin,
		DownloadInfo:    downloadInfo,
		ArchivePath:     "path",
		ObjectStoreUUID: "1234",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) TestNormalizeControllerConstraints(c *tc.C) {
	got, err := normalizeControllerConstraints(constraints.Value{}, "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.Arch, tc.NotNil)
	c.Check(*got.Arch, tc.Equals, "amd64")

	got, err = normalizeControllerConstraints(constraints.Value{Arch: new("arm64")}, "arm64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.Arch, tc.NotNil)
	c.Check(*got.Arch, tc.Equals, "arm64")
}

func (s *deployerCAASSuite) TestNormalizeControllerConstraintsRejectsMismatchedArchitecture(c *tc.C) {
	_, err := normalizeControllerConstraints(constraints.Value{Arch: new("arm64")}, "amd64")
	c.Assert(err, tc.ErrorMatches, "arch in platform and constraints for controller do not match")
}

func (s *deployerCAASSuite) TestEnsureControllerApplicationServiceAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	info := s.controllerCharmInfo()
	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)

	unitName := unit.Name("controller/0")

	providerAddress := network.ProviderAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopeMachineLocal,
			},
		},
		{
			MachineAddress: network.MachineAddress{
				Value: "203.0.113.1",
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			},
		},
	}

	s.caasApplicationService.EXPECT().UpdateK8sService(gomock.Any(), bootstrap.ControllerApplicationName, controllerProviderID(unitName), providerAddress).Return(nil)
	s.caasApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) TestEnsureControllerApplicationSetsFQDN(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	const fqdn = "controller-0.controller-service-endpoints.controller-foo.svc.cluster.local"
	cfg.ControllerFQDN = fqdn
	info := s.controllerCharmInfo()
	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)

	unitName := unit.Name("controller/0")

	s.caasApplicationService.EXPECT().UpdateK8sService(gomock.Any(), bootstrap.ControllerApplicationName, controllerProviderID(unitName), gomock.Any()).Return(nil)
	// The controller FQDN is persisted in the same flow that upserts the k8s
	// pod (provider id), i.e. via UpdateCAASUnit.
	s.caasApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
		FQDN:       new(fqdn),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) TestEnsureControllerApplicationCreationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := s.controllerCharmInfo()
	expectedErr := errors.New("cannot create controller application")
	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", expectedErr)

	err := s.newDeployer(c).EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *deployerCAASSuite) TestEnsureControllerApplicationRetriesCompletion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	cfg.ControllerFQDN = "controller-0.controller-service-endpoints.controller-foo.svc.cluster.local"
	info := s.controllerCharmInfo()
	deployer := s.newDeployerWithConfig(c, cfg)

	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)
	expectedErr := errors.New("cannot update controller service")
	s.expectControllerApplicationCompletion(cfg, expectedErr)

	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)

	// Creation succeeded on the first attempt, so the retry must complete
	// setup even though the application already exists.
	s.caasApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", errors.Annotate(applicationerrors.ApplicationAlreadyExists, "controller"))
	s.expectControllerApplicationCompletion(cfg, nil)

	err = deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) expectControllerApplicationCompletion(cfg CAASDeployerConfig, serviceErr error) {
	unitName := unit.Name("controller/0")
	params := applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
	}
	if cfg.ControllerFQDN != "" {
		params.FQDN = &cfg.ControllerFQDN
	}
	gomock.InOrder(
		s.caasApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, params).Return(nil),
		s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword).Return(nil),
		s.caasApplicationService.EXPECT().UpdateK8sService(
			gomock.Any(), bootstrap.ControllerApplicationName, "controller-0", cfg.BootstrapAddresses,
		).Return(serviceErr),
	)
}

func (s *deployerCAASSuite) newDeployer(c *tc.C) *CAASDeployer {
	return s.newDeployerWithConfig(c, s.newConfig(c))
}

func (s *deployerCAASSuite) newDeployerWithConfig(c *tc.C, cfg CAASDeployerConfig) *CAASDeployer {
	deployer, err := NewCAASDeployer(cfg)
	c.Assert(err, tc.IsNil)
	return deployer
}

func (s *deployerCAASSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.clock = NewMockClock(ctrl)
	s.serviceManager = NewMockServiceManager(ctrl)

	return ctrl
}

func (s *deployerCAASSuite) newConfig(c *tc.C) CAASDeployerConfig {
	return CAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		ApplicationService: s.caasApplicationService,
		UnitPassword:       uuid.MustNewUUID().String(),
		ServiceManager:     s.serviceManager,
	}
}
