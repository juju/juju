// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
)

func TestMigrateSuite(t *testing.T) {
	tc.Run(t, &migrateSuite{})
}

type migrateSuite struct {
	testhelpers.IsolationSuite

	agentService   *mocks.MockModelAgentService
	machineService *mocks.MockMachineService
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju3(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineNames := []machine.Name{"0", "1", "2"}
	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return(machineNames, nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@24.04"), nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("2")).Return(base.MustParseBaseFromString("ubuntu@20.04"), nil)

	validators := upgradevalidation.ValidatorsForModelMigrationSource()
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: s.agentService,
		MachineService:    s.machineService,
	}
	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices, validators...)
	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju3Failed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineNames := []machine.Name{"0", "1", "2"}
	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return(machineNames, nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@24.04"), nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("2")).Return(base.MustParseBaseFromString("ubuntu@18.04"), nil)

	validators := upgradevalidation.ValidatorsForModelMigrationSource()
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: s.agentService,
		MachineService:    s.machineService,
	}
	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices, validators...)
	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers.String(), tc.Contains, "unsupported base")
}

func (s *migrateSuite) initializeMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentService = mocks.NewMockModelAgentService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)
	c.Cleanup(func() {
		s.agentService = nil
		s.machineService = nil
	})
	return ctrl
}

func (s *migrateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.initializeMocks(c)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	return ctrl
}
