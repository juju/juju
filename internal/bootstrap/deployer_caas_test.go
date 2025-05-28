// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	network "github.com/juju/juju/core/network"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/internal/uuid"
)

type deployerCAASSuite struct {
	baseSuite
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

func (s *deployerCAASSuite) TestControllerAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				ProviderID: network.Id("0"),
			},
		},
	}, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the test picks the first address.

	cfg := s.newConfig(c)

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				ProviderID: network.Id("0"),
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				ProviderID: network.Id("1"),
			},
		},
	}, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddressesScopeNonLocal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we always pick local over non-local

	cfg := s.newConfig(c)

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.2",
				},
				ProviderID: network.Id("0"),
			},
			{
				MachineAddress: network.MachineAddress{
					Value: "2.201.120.241",
				},
				ProviderID: network.Id("1"),
			},
		},
	}, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.2:0")
}

func (s *deployerCAASSuite) TestControllerAddressScopeNonLocal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we return the non scoped local address if we don't have
	// any local addresses.

	cfg := s.newConfig(c)

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "2.201.120.241",
				},
				ProviderID: network.Id("0"),
			},
		},
	}, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "2.201.120.241:0")
}

func (s *deployerCAASSuite) TestControllerAddressNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{},
	}, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	_, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorMatches, "k8s controller service .* address not provisioned")
}

func (s *deployerCAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.DeepEquals, version.DefaultSupportedLTSBase())
}

func (s *deployerCAASSuite) TestCompleteProcessWithSpaceInAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	unitName := unit.Name("controller/0")

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				SpaceName:  network.SpaceName("mgmt-space"),
				ProviderID: network.Id("0"),
			},
		},
	}, nil)
	// If there's a space in the provider address, then we override it with
	// the alpha space ID.
	alphaAddresses := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
			SpaceID: network.AlphaSpaceId,
		},
	}
	s.caasApplicationService.EXPECT().UpdateCloudService(gomock.Any(), bootstrap.ControllerApplicationName, controllerProviderID(unitName), alphaAddresses).Return(nil)
	s.caasApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("controller-0"),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.CompleteCAASProcess(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *deployerCAASSuite) TestCompleteCAASProcess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	unitName := unit.Name("controller/0")

	s.serviceManager.EXPECT().GetService(gomock.Any(), k8sconstants.JujuControllerStackName, true).Return(&caas.Service{
		Addresses: network.ProviderAddresses{
			{
				MachineAddress: network.MachineAddress{
					Value: "10.0.0.1",
				},
				ProviderID: network.Id("0"),
			},
		},
	}, nil)
	alphaAddresses := network.SpaceAddresses{
		{
			MachineAddress: network.MachineAddress{
				Value: "10.0.0.1",
			},
		},
	}
	s.caasApplicationService.EXPECT().UpdateCloudService(gomock.Any(), bootstrap.ControllerApplicationName, controllerProviderID(unitName), alphaAddresses).Return(nil)
	s.caasApplicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("controller-0"),
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
