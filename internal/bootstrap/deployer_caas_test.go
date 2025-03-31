// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
)

type deployerCAASSuite struct {
	baseSuite

	cloudService       *MockCloudService
	cloudServiceGetter *MockCloudServiceGetter
	operationApplier   *MockOperationApplier
}

var _ = gc.Suite(&deployerCAASSuite{})

func (s *deployerCAASSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig(c)
	cfg.CloudServiceGetter = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.OperationApplier = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerCAASSuite) TestControllerAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("10.0.0.1"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that the test picks the first address.

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("10.0.0.1", "10.0.0.2"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "10.0.0.1:0")
}

func (s *deployerCAASSuite) TestControllerAddressMultipleAddressesScopeNonLocal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we always pick local over non-local

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("2.201.120.241", "10.0.0.2"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "10.0.0.2:0")
}

func (s *deployerCAASSuite) TestControllerAddressScopeNonLocal(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure that we return the non scoped local address if we don't have
	// any local addresses.

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses("2.201.120.241"))
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "2.201.120.241:0")
}

func (s *deployerCAASSuite) TestControllerAddressNoAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	s.cloudService.EXPECT().Addresses().Return(network.NewSpaceAddresses())
	s.cloudServiceGetter.EXPECT().CloudService(cfg.ControllerConfig.ControllerUUID()).Return(s.cloudService, nil)

	deployer := s.newDeployerWithConfig(c, cfg)
	address, err := deployer.ControllerAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "")
}

func (s *deployerCAASSuite) TestControllerCharmBase(c *gc.C) {
	defer s.setupMocks(c).Finish()

	deployer := s.newDeployer(c)
	base, err := deployer.ControllerCharmBase()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, gc.DeepEquals, version.DefaultSupportedLTSBase())
}

func (s *deployerCAASSuite) TestCompleteProcess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)

	op := &state.UpdateUnitOperation{}

	s.unit.EXPECT().UnitTag().Return(names.NewUnitTag("controller/0")).AnyTimes()
	s.unit.EXPECT().UpdateOperation(state.UnitUpdateProperties{
		ProviderId: ptr("controller-0"),
	}).Return(op)
	s.operationApplier.EXPECT().ApplyOperation(op).Return(nil)
	s.unit.EXPECT().SetPassword(cfg.UnitPassword).Return(nil)

	s.applicationService.EXPECT().UpdateCAASUnit(gomock.Any(), unit.Name("controller/0"), applicationservice.UpdateCAASUnitParams{
		ProviderID: ptr("controller-0"),
	})
	s.passwordService.EXPECT().SetUnitPassword(gomock.Any(), unit.Name("controller/0"), cfg.UnitPassword)

	deployer := s.newDeployerWithConfig(c, cfg)
	err := deployer.CompleteProcess(context.Background(), s.unit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *deployerCAASSuite) newDeployer(c *gc.C) *CAASDeployer {
	return s.newDeployerWithConfig(c, s.newConfig(c))
}

func (s *deployerCAASSuite) newDeployerWithConfig(c *gc.C, cfg CAASDeployerConfig) *CAASDeployer {
	deployer, err := NewCAASDeployer(cfg)
	c.Assert(err, gc.IsNil)
	return deployer
}

func (s *deployerCAASSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.cloudService = NewMockCloudService(ctrl)
	s.cloudServiceGetter = NewMockCloudServiceGetter(ctrl)
	s.operationApplier = NewMockOperationApplier(ctrl)

	return ctrl
}

func (s *deployerCAASSuite) newConfig(c *gc.C) CAASDeployerConfig {
	return CAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(c),
		CloudServiceGetter: s.cloudServiceGetter,
		OperationApplier:   s.operationApplier,
		UnitPassword:       uuid.MustNewUUID().String(),
	}
}
