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

func (s *deployerCAASSuite) TestAddCAASControllerApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()
	s.clock.EXPECT().Now().Return(now).AnyTimes()

	cfg := s.newConfig(c)
	cfg.Clock = s.clock

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
			Constraints:  constraints.Value{},
			IsController: true,
		},
		applicationservice.AddUnitArg{},
	)

	deployer := s.newDeployerWithConfig(c, cfg)

	downloadInfo := &corecharm.DownloadInfo{
		CharmhubIdentifier: "abcd",
		DownloadURL:        "https://inferi.com",
		DownloadSize:       42,
	}
	err := deployer.AddCAASControllerApplication(c.Context(), DeployCharmInfo{
		URL:             charm.MustParseURL(curl),
		Charm:           s.charm,
		Origin:          &origin,
		DownloadInfo:    downloadInfo,
		ArchivePath:     "path",
		ObjectStoreUUID: "1234",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) TestCompleteCAASProcess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

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
	err := deployer.CompleteCAASProcess(c.Context())
	c.Assert(err, tc.ErrorIsNil)
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
