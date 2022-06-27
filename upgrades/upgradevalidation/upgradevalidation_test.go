// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/upgrades/upgradevalidation"
	"github.com/juju/juju/upgrades/upgradevalidation/mocks"
)

var _ = gc.Suite(&upgradeValidationSuite{})

type upgradeValidationSuite struct {
	jujutesting.IsolationSuite
}

func (s *upgradeValidationSuite) TestModelUpgradeBlockers(c *gc.C) {
	blockers1 := upgradevalidation.NewModelUpgradeBlockers(
		"controller",
		*upgradevalidation.NewBlocker("model migration is in process"),
		*upgradevalidation.NewBlocker("unexpected upgrade series lock found"),
	)
	for i := 1; i < 5; i++ {
		blockers := upgradevalidation.NewModelUpgradeBlockers(
			fmt.Sprintf("model-%d", i),
			*upgradevalidation.NewBlocker("unexpected upgrade series lock found"),
			*upgradevalidation.NewBlocker("model migration is in process"),
		)
		blockers1.Join(blockers)
	}
	c.Assert(blockers1.String(), gc.Equals, `
model "controller":
	model migration is in process
	unexpected upgrade series lock found
model "model-1":
	unexpected upgrade series lock found
	model migration is in process
model "model-2":
	unexpected upgrade series lock found
	model migration is in process
model "model-3":
	unexpected upgrade series lock found
	model migration is in process
model "model-4":
	unexpected upgrade series lock found
	model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck("", statePool, state, model,
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return nil, errors.New("server is unreachable")
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, gc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, gc.IsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	state := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model.EXPECT().Name().Return("model-1"),
	)

	checker := upgradevalidation.NewModelUpgradeCheck(coretesting.ModelTag.Id(), statePool, state, model,
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(modelUUID string, pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("unexpected upgrade series lock found"), nil
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers.String(), gc.Equals, `
model "admin/model-1":
	model migration is in process
	unexpected upgrade series lock found`[1:])
}

func (s *upgradeValidationSuite) TestCheckNoWinMachinesForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockState(ctrl)
	gomock.InOrder(
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(1, nil),
	)

	blocker, err := upgradevalidation.CheckNoWinMachinesForModel("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckNoWinMachinesForModel("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `model hosts 1 windows machine(s)`)
}

func (s *upgradeValidationSuite) TestGetCheckUpgradeSeriesLockForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockState(ctrl)
	gomock.InOrder(
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
		state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
		state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
	)

	blocker, err := upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(true)("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `unexpected upgrade series lock found`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgrades.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.30"),
	})

	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.29"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(version.MustParse("3.0-beta1"))("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `upgrade current version "2.9.29" to at least "2.9.30" before upgrading to "3.0-beta1"`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(version.MustParse("3.0-beta1"))("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(version.MustParse("1.1.1"))("", nil, nil, model)
	c.Assert(err, gc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(version.MustParse("4.1.1"))("", nil, nil, model)
	c.Assert(err, gc.ErrorMatches, `unknown version "4.1.1"`)
	c.Assert(blocker, gc.IsNil)
}

func (s *upgradeValidationSuite) TestCheckModelMigrationModeForControllerUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().MigrationMode().Return(state.MigrationModeNone),
		model.EXPECT().MigrationMode().Return(state.MigrationModeImporting),
		model.EXPECT().MigrationMode().Return(state.MigrationModeExporting),
	)

	blocker, err := upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `model is under "importing" mode, upgrade blocked`)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `model is under "exporting" mode, upgrade blocked`)
}

func (s *upgradeValidationSuite) TestCheckMongoStatusForControllerUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := mocks.NewMockState(ctrl)
	gomock.InOrder(
		state.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					Address: "1.1.1.1",
					State:   replicaset.PrimaryState,
				},
				{
					Id:      2,
					Address: "2.2.2.2",
					State:   replicaset.SecondaryState,
				},
				{
					Id:      3,
					Address: "3.3.3.3",
					State:   replicaset.SecondaryState,
				},
			},
		}, nil),
		state.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					Address: "1.1.1.1",
					State:   replicaset.RecoveringState,
				},
				{
					Id:      2,
					Address: "2.2.2.2",
					State:   replicaset.FatalState,
				},
				{
					Id:      3,
					Address: "3.3.3.3",
					State:   replicaset.Startup2State,
				},
				{
					Id:      4,
					Address: "4.4.4.4",
					State:   replicaset.UnknownState,
				},
				{
					Id:      5,
					Address: "5.5.5.5",
					State:   replicaset.ArbiterState,
				},
				{
					Id:      6,
					Address: "6.6.6.6",
					State:   replicaset.DownState,
				},
				{
					Id:      7,
					Address: "7.7.7.7",
					State:   replicaset.RollbackState,
				},
				{
					Id:      8,
					Address: "8.8.8.8",
					State:   replicaset.ShunnedState,
				},
			},
		}, nil),
	)

	blocker, err := upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, state, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `unable to upgrade, database node 1 (1.1.1.1) has state RECOVERING, node 2 (2.2.2.2) has state FATAL, node 3 (3.3.3.3) has state STARTUP2, node 4 (4.4.4.4) has state UNKNOWN, node 5 (5.5.5.5) has state ARBITER, node 6 (6.6.6.6) has state DOWN, node 7 (7.7.7.7) has state ROLLBACK, node 8 (8.8.8.8) has state SHUNNED`)
}

func (s *upgradeValidationSuite) TestCheckMongoVersionForControllerModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	pool := mocks.NewMockStatePool(ctrl)
	gomock.InOrder(
		pool.EXPECT().MongoVersion().Return(`4.4`, nil),
		pool.EXPECT().MongoVersion().Return(`4.3`, nil),
	)

	blocker, err := upgradevalidation.CheckMongoVersionForControllerModel("", pool, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckMongoVersionForControllerModel("", pool, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `mongo version has to be "4.4" at least, but current version is "4.3"`)
}
