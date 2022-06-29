// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades/upgradevalidation"
	"github.com/juju/juju/upgrades/upgradevalidation/mocks"
)

func (s *upgradeValidationSuite) TestValidatorsForModelMigrationSourceJuju3(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	gomock.InOrder(
		// - check agent version;
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.32"), nil),
		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
		// - check if the model has win machines;
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	)

	targetVersion := version.MustParse("3.0-beta1")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), statePool, state, model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *upgradeValidationSuite) TestValidatorsForModelMigrationSourceJuju2(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	gomock.InOrder(
		// - check agent version;
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.32"), nil),
		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
	)

	targetVersion := version.MustParse("2.9.99")
	validators := upgradevalidation.ValidatorsForModelMigrationSource(targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), statePool, state, model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}
