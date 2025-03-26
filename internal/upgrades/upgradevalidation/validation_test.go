// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&upgradeValidationSuite{})

type upgradeValidationSuite struct {
	jujutesting.IsolationSuite
}

func (s *upgradeValidationSuite) TestModelUpgradeBlockers(c *gc.C) {
	blockers1 := upgradevalidation.NewModelUpgradeBlockers(
		"controller",
		*upgradevalidation.NewBlocker("model migration is in process"),
	)
	for i := 1; i < 5; i++ {
		blockers := upgradevalidation.NewModelUpgradeBlockers(
			fmt.Sprintf("model-%d", i),
			*upgradevalidation.NewBlocker("model migration is in process"),
		)
		blockers1.Join(blockers)
	}
	c.Assert(blockers1.String(), gc.Equals, `
"controller":
- model migration is in process
"model-1":
- model migration is in process
"model-2":
- model migration is in process
"model-3":
- model migration is in process
"model-4":
- model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	agentVersion := mocks.NewMockModelAgentService(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck(statePool, st, model, agentVersion,
		func(pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
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
	agentService := mocks.NewMockModelAgentService(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck(statePool, st, model, agentService,
		func(pool upgradevalidation.StatePool, st upgradevalidation.State, model upgradevalidation.Model, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers.String(), gc.Equals, `
"admin/model-1":
- model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestCheckForDeprecatedUbuntuSeriesForModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	st := mocks.NewMockState(ctrl)
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(map[string]int{"ubuntu@20.04": 1, "ubuntu@22.04": 1, "ubuntu@24.04": 2}, nil)
	st.EXPECT().AllMachinesCount().Return(5, nil)

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel(nil, st, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForControllerModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.30"),
	})

	agentService := mocks.NewMockModelAgentService(ctrl)
	gomock.InOrder(
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse("2.9.29"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, nil, nil, agentService)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, nil, nil, agentService)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, nil, nil, agentService)
	c.Assert(err, gc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		version.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, nil, nil, agentService)
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

	blocker, err := upgradevalidation.CheckModelMigrationModeForControllerUpgrade(nil, nil, model, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade(nil, nil, model, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `model is under "importing" mode, upgrade blocked`)

	blocker, err = upgradevalidation.CheckModelMigrationModeForControllerUpgrade(nil, nil, model, nil)
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

	blocker, err := upgradevalidation.CheckMongoStatusForControllerUpgrade(nil, st, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckMongoStatusForControllerUpgrade(nil, st, nil, nil)
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

	blocker, err := upgradevalidation.CheckMongoVersionForControllerModel(pool, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.CheckMongoVersionForControllerModel(pool, nil, nil, nil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)(nil, nil, nil, nil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(environscloudspec.CloudSpec{Type: "foo"})(nil, nil, nil, nil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)(nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.NotNil)
	c.Assert(blocker.Error(), gc.Equals, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}
