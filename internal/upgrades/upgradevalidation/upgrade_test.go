// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
)

func (s *upgradeValidationSuite) TestValidatorsForControllerUpgradeJuju3(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]semversion.Number{
		3: semversion.MustParse("2.9.1"),
	})

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	agentVersion := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)

	machineNames := []machine.Name{"0", "1", "2"}
	machineService.EXPECT().AllMachineNames(gomock.Any()).Return(machineNames, nil).Times(2)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@24.04"), nil).Times(2)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil).Times(2)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("2")).Return(base.MustParseBaseFromString("ubuntu@20.04"), nil).Times(2)

	// 1. Check controller model.
	// - check agent version;
	agentVersion.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("3.666.1"), nil)
	// 2. Check hosted models.
	// - check agent version;
	agentVersion.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.1"), nil)
	//  - check if model migration is ongoing;

	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentVersion,
		MachineService:    machineService,
	}

	targetVersion := semversion.MustParse("3.666.2")
	validators := upgradevalidation.ValidatorsForControllerModelUpgrade(targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices, validators...)
	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)

	validators = upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion)
	checker = upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices, validators...)
	blockers, err = checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}

func (s *upgradeValidationSuite) TestValidatorsForModelUpgradeJuju3(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	agentService := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)

	machineNames := []machine.Name{"0", "1", "2"}
	machineService.EXPECT().AllMachineNames(gomock.Any()).Return(machineNames, nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@24.04"), nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("2")).Return(base.MustParseBaseFromString("ubuntu@20.04"), nil)

	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentService,
		MachineService:    machineService,
	}

	targetVersion := semversion.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelUpgrade(false, targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices, validators...)
	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}
