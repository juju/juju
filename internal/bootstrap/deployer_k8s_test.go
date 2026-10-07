// Copyright 2026 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/uuid"
)

type deployerK8sSuite struct {
	baseSuite
	clock          *MockClock
	serviceManager *MockServiceManager
}

func TestDeployerK8sSuite(t *testing.T) {
	tc.Run(t, &deployerK8sSuite{})
}

func (s *deployerK8sSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, tc.IsNil)

	cfg = s.newConfig(c)
	cfg.ServiceManager = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerK8sSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.DeepEquals, version.DefaultSupportedLTSBase())
}

func (s *deployerK8sSuite) TestEnsureControllerApplication(c *tc.C) {
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

	s.k8sApplicationService.EXPECT().CreateCAASApplication(
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
	s.expectControllerApplicationExposure()

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

func (s *deployerK8sSuite) TestNormalizeControllerConstraints(c *tc.C) {
	got, err := normalizeControllerConstraints(constraints.Value{}, "amd64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.Arch, tc.NotNil)
	c.Check(*got.Arch, tc.Equals, "amd64")

	got, err = normalizeControllerConstraints(constraints.Value{Arch: new("arm64")}, "arm64")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got.Arch, tc.NotNil)
	c.Check(*got.Arch, tc.Equals, "arm64")
}

func (s *deployerK8sSuite) TestNormalizeControllerConstraintsRejectsMismatchedArchitecture(c *tc.C) {
	_, err := normalizeControllerConstraints(constraints.Value{Arch: new("arm64")}, "amd64")
	c.Assert(err, tc.ErrorMatches, "arch \"arm64\" in constraints does not match controller charm platform arch \"amd64\"")
}

func (s *deployerK8sSuite) TestEnsureControllerApplicationServiceAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	info := s.controllerCharmInfo()
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)

	unitName := unit.Name("controller/0")

	providerAddress := network.ProviderAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
				Type:  network.IPv4Address,
				Scope: network.ScopeCloudLocal,
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

	s.serviceManager.EXPECT().GetService(gomock.Any(), bootstrap.ControllerApplicationName, true).Return(&caas.Service{
		Id: "service-uid", Addresses: providerAddress,
	}, nil)
	s.k8sApplicationService.EXPECT().UpdateK8sService(gomock.Any(), bootstrap.ControllerApplicationName, "service-uid", providerAddress).Return(nil)
	s.k8sApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)
	s.expectControllerApplicationExposure()

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerK8sSuite) TestEnsureControllerApplicationSetsFQDN(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	const fqdn = "controller-0.controller-service-endpoints.controller-foo.svc.cluster.local"
	cfg.ControllerFQDN = fqdn
	info := s.controllerCharmInfo()
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)

	unitName := unit.Name("controller/0")

	s.serviceManager.EXPECT().GetService(gomock.Any(), bootstrap.ControllerApplicationName, true).Return(&caas.Service{
		Id: "service-uid", Addresses: cfg.BootstrapAddresses,
	}, nil)
	s.k8sApplicationService.EXPECT().UpdateK8sService(gomock.Any(), bootstrap.ControllerApplicationName, "service-uid", cfg.BootstrapAddresses).Return(nil)
	// The controller FQDN is persisted in the same flow that upserts the k8s
	// pod (provider id), i.e. via UpdateCAASUnit.
	s.k8sApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
		FQDN:       new(fqdn),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)
	s.expectControllerApplicationExposure()

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerK8sSuite) TestCompleteControllerApplicationServiceReadFails(c *tc.C) {
	defer s.setupMocks(c).Finish()
	cfg := s.newConfig(c)
	deployer := s.newDeployerWithConfig(c, cfg)
	expectedErr := errors.New("provider unavailable")
	for _, test := range []struct {
		service *caas.Service
		err     error
	}{
		{err: expectedErr},
		{},
		{service: &caas.Service{}},
	} {
		s.k8sApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unit.Name("controller/0"), gomock.Any()).Return(nil)
		s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unit.Name("controller/0"), cfg.UnitPassword).Return(nil)
		s.serviceManager.EXPECT().GetService(gomock.Any(), bootstrap.ControllerApplicationName, true).Return(test.service, test.err)
		err := deployer.completeControllerApplication(c.Context())
		if test.err != nil {
			c.Assert(err, tc.ErrorIs, expectedErr)
		} else {
			c.Assert(err, tc.ErrorMatches, "controller API service has not been provisioned")
		}
	}
}

func (s *deployerK8sSuite) TestEnsureControllerApplicationCreationFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	info := s.controllerCharmInfo()
	expectedErr := errors.New("cannot create controller application")
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", expectedErr)

	err := s.newDeployer(c).EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *deployerK8sSuite) TestEnsureControllerApplicationRetriesCompletion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	cfg.ControllerFQDN = "controller-0.controller-service-endpoints.controller-foo.svc.cluster.local"
	info := s.controllerCharmInfo()
	deployer := s.newDeployerWithConfig(c, cfg)

	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)
	expectedErr := errors.New("cannot update controller service")
	s.expectControllerApplicationCompletion(cfg, expectedErr)

	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)

	// Creation succeeded on the first attempt, so the retry must complete
	// setup even though the application already exists.
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", errors.Annotate(applicationerrors.ApplicationAlreadyExists, "controller"))
	s.expectControllerApplicationCompletion(cfg, nil)
	s.expectControllerApplicationExposure()

	err = deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerK8sSuite) TestEnsureControllerApplicationRetriesExposure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	info := s.controllerCharmInfo()
	deployer := s.newDeployerWithConfig(c, cfg)
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", nil)
	s.expectControllerApplicationCompletion(cfg, nil)
	expectedErr := errors.New("cannot expose controller application")
	gomock.InOrder(
		s.applicationService.EXPECT().IsApplicationExposed(gomock.Any(), bootstrap.ControllerApplicationName).Return(false, nil),
		s.applicationService.EXPECT().MergeExposeSettings(gomock.Any(), bootstrap.ControllerApplicationName, nil).Return(expectedErr),
	)

	err := deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIs, expectedErr)

	// Unit and service setup completed on the first attempt. The retry must
	// still expose the existing application before bootstrap can finish.
	s.k8sApplicationService.EXPECT().CreateCAASApplication(
		gomock.Any(), bootstrap.ControllerApplicationName, s.charm,
		*info.Origin, gomock.Any(), gomock.Any(),
	).Return("", applicationerrors.ApplicationAlreadyExists)
	s.expectControllerApplicationCompletion(cfg, nil)
	s.expectControllerApplicationExposure()

	err = deployer.EnsureControllerApplication(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerK8sSuite) expectControllerApplicationCompletion(cfg K8sDeployerConfig, serviceErr error) {
	serviceAddresses := network.ProviderAddresses{
		network.NewMachineAddress("10.0.0.10", network.WithScope(network.ScopeCloudLocal)).AsProviderAddress(),
		network.NewMachineAddress("api.example.com", network.WithScope(network.ScopePublic)).AsProviderAddress(),
	}
	unitName := unit.Name("controller/0")
	params := applicationservice.UpdateCAASUnitParams{
		ProviderID: new("controller-0"),
	}
	if cfg.ControllerFQDN != "" {
		params.FQDN = &cfg.ControllerFQDN
	}
	gomock.InOrder(
		s.k8sApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, params).Return(nil),
		s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword).Return(nil),
		s.serviceManager.EXPECT().GetService(gomock.Any(), bootstrap.ControllerApplicationName, true).Return(&caas.Service{
			Id: "controller-service-uid", Addresses: serviceAddresses,
		}, nil),
		s.k8sApplicationService.EXPECT().UpdateK8sService(
			gomock.Any(), bootstrap.ControllerApplicationName, "controller-service-uid", serviceAddresses,
		).Return(serviceErr),
	)
}

func (s *deployerK8sSuite) newDeployer(c *tc.C) *K8sDeployer {
	return s.newDeployerWithConfig(c, s.newConfig(c))
}

func (s *deployerK8sSuite) newDeployerWithConfig(c *tc.C, cfg K8sDeployerConfig) *K8sDeployer {
	deployer, err := NewK8sDeployer(cfg)
	c.Assert(err, tc.IsNil)
	return deployer
}

func (s *deployerK8sSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.clock = NewMockClock(ctrl)
	s.serviceManager = NewMockServiceManager(ctrl)

	return ctrl
}

func (s *deployerK8sSuite) newConfig(c *tc.C) K8sDeployerConfig {
	return K8sDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		ApplicationService: s.k8sApplicationService,
		UnitPassword:       uuid.MustNewUUID().String(),
		ServiceManager:     s.serviceManager,
	}
}
