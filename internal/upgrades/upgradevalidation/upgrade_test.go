// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
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

	ctrlState := mocks.NewMockState(ctrl)
	agentVersion := mocks.NewMockModelAgentService(ctrl)

	state1 := mocks.NewMockState(ctrl)

	// 1. Check controller model.
	// - check agent version;
	agentVersion.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("3.666.1"), nil)
	ctrlState.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	ctrlState.EXPECT().AllMachinesCount().Return(0, nil)
	// 2. Check hosted models.
	// - check agent version;
	agentVersion.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.1"), nil)
	//  - check if model migration is ongoing;
	state1.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	state1.EXPECT().AllMachinesCount().Return(0, nil)

	targetVersion := semversion.MustParse("3.666.2")
	validators := upgradevalidation.ValidatorsForControllerModelUpgrade(targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck(ctrlState, "test-model", agentVersion, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)

	validators = upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion)
	checker = upgradevalidation.NewModelUpgradeCheck(state1, "test-model", agentVersion, validators...)
	blockers, err = checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}

func (s *upgradeValidationSuite) TestValidatorsForModelUpgradeJuju3(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	st := mocks.NewMockState(ctrl)
	agentService := mocks.NewMockModelAgentService(ctrl)

	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	st.EXPECT().AllMachinesCount().Return(0, nil)

	targetVersion := semversion.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelUpgrade(false, targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck(st, "test-model", agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}
