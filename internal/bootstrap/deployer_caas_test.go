// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/uuid"
)

type deployerCAASSuite struct {
	baseSuite

	cloudService       *MockCloudService
	cloudServiceGetter *MockCloudServiceGetter
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
	cfg.CloudServiceGetter = nil
	err = cfg.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *deployerCAASSuite) TestControllerAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("10.0.0.1"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the test picks the first address.

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddressesScopeNonLocal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we always pick local over non-local

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("2.201.120.241", "10.0.0.2"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

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

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("2.201.120.241"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "2.201.120.241:0")
}

func (s *deployerCAASSuite) TestControllerAddressNoAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses())
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(address, tc.Equals, "")
}

func (s *deployerCAASSuite) TestControllerCharmBase(c *tc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(base, tc.DeepEquals, version.DefaultSupportedLTSBase())
}

func (s *deployerCAASSuite) TestCompleteProcess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	unitName := unit.Name("controller/0")

	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("controller-0"),
	})
	s.agentPasswordService.EXPECT().SetUnitPassword(gomock.Any(), unitName, cfg.UnitPassword)

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.CompleteProcess(c.Context(), unitName)
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

	s.cloudService = NewMockCloudService(ctrl)
	s.cloudServiceGetter = NewMockCloudServiceGetter(ctrl)

	return ctrl
}

func (s *deployerCAASSuite) newConfig(c *tc.C) CAASDeployerConfig {
	return CAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		CloudServiceGetter: s.cloudServiceGetter,
		UnitPassword:       uuid.MustNewUUID().String(),
	}
}
