// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
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
"controller":
- model migration is in process
- unexpected upgrade series lock found
"model-1":
- unexpected upgrade series lock found
- model migration is in process
"model-2":
- unexpected upgrade series lock found
- model migration is in process
"model-3":
- unexpected upgrade series lock found
- model migration is in process
"model-4":
- unexpected upgrade series lock found
- model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck("", statePool, st, model,
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
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	model.EXPECT().Owner().Return(names.NewUserTag("admin"))
	model.EXPECT().Name().Return("model-1")

	checker := upgradevalidation.NewModelUpgradeCheck(coretesting.ModelTag.Id(), statePool, st, model,
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
"admin/model-1":
- model migration is in process
- unexpected upgrade series lock found`[1:])
}

func (s *upgradeValidationSuite) TestCheckForDeprecatedUbuntuSeriesForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", ubuntuVersions)).Return(map[string]int{"xenial": 1, "vivid": 2, "trusty": 3}, nil)

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel("", nil, st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `the model hosts deprecated ubuntu machine(s): trusty(3) vivid(2) xenial(1)`)
}

func (s *upgradeValidationSuite) TestGetCheckUpgradeSeriesLockForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	gomock.InOrder(
		st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),
		st.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
		st.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),
	)

	blocker, err := upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(true)("", nil, st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckUpgradeSeriesLockForModel(false)("", nil, st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `unexpected upgrade series lock found`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForControllerModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.30"),
	})

	model := mocks.NewMockModel(ctrl)
	gomock.InOrder(
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.29"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
		model.EXPECT().AgentVersion().Return(version.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, gc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)("", nil, nil, model)
	c.Assert(err, gc.ErrorMatches, `upgrading controller to "4.1.1" is not supported from "2.9.31"`)
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

	st := mocks.NewMockState(ctrl)
	gomock.InOrder(
		st.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
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
		st.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
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

	blocker, err := upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckMongoStatusForControllerUpgrade("", nil, st, nil)
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

func (s *upgradeValidationSuite) assertGetCheckForLXDVersion(c *gc.C, cloudType string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: cloudType}}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)("", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionLXD(c *gc.C) {
	s.assertGetCheckForLXDVersion(c, "lxd")
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionLocalhost(c *gc.C) {
	s.assertGetCheckForLXDVersion(c, "localhost")
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionSkippedForNonLXDCloud(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	blocker, err := upgradevalidation.GetCheckForLXDVersion(environscloudspec.CloudSpec{Type: "foo"})("", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)
}

func (s *upgradeValidationSuite) TestGetCheckForLXDVersionFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)

	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)("", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.NotNil)
	c.Assert(blocker.Error(), gc.Equals, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}
