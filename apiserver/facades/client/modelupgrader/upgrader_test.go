// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v3"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	modelmocks "github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	"github.com/juju/juju/apiserver/facades/client/modelupgrader"
	"github.com/juju/juju/apiserver/facades/client/modelupgrader/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades/upgradevalidation"
)

type modelManagerUpgradeSuite struct {
	jujutesting.IsolationSuite

	adminUser   names.UserTag
	authoriser  apiservertesting.FakeAuthorizer
	callContext context.ProviderCallContext

	statePool        *mocks.MockStatePool
	toolsFinder      *mocks.MockToolsFinder
	bootstrapEnviron *mocks.MockBootstrapEnviron
	blockChecker     *modelmocks.MockBlockCheckerInterface
}

var _ = gc.Suite(&modelManagerUpgradeSuite{})

func (s *modelManagerUpgradeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *modelManagerUpgradeSuite) getModelUpgraderAPI(c *gc.C) (*gomock.Controller, *modelupgrader.ModelUpgraderAPI) {
	ctrl := gomock.NewController(c)
	s.statePool = mocks.NewMockStatePool(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.bootstrapEnviron = mocks.NewMockBootstrapEnviron(ctrl)
	s.blockChecker = modelmocks.NewMockBlockCheckerInterface(ctrl)

	api, err := modelupgrader.NewModelUpgraderAPI(
		coretesting.ControllerTag,
		s.statePool,
		s.toolsFinder,
		func() (environs.BootstrapEnviron, error) {
			return s.bootstrapEnviron, nil
		},
		s.blockChecker, s.authoriser, s.callContext,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl, api
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelWithInvalidModelTag(c *gc.C) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	err := api.UpgradeModel(params.UpgradeModel{ModelTag: "!!!"})
	c.Assert(err, gc.ErrorMatches, `"!!!" is not a valid tag`)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelWithModelWithNoPermission(c *gc.C) {
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("user"),
	}
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:  coretesting.ModelTag.String(),
			ToVersion: version.MustParse("3.0.0"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelWithChangeNotAllowed(c *gc.C) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(errors.Errorf("the operation has been blocked"))

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:  coretesting.ModelTag.String(),
			ToVersion: version.MustParse("3.0.0"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `the operation has been blocked`)
}

func (s *modelManagerUpgradeSuite) assertUpgradeModelForControllerModelJuju3(c *gc.C, dryRun bool) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	ctrlModelTag := coretesting.ModelTag
	model1ModelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctrlModel := mocks.NewMockModel(ctrl)
	model1 := mocks.NewMockModel(ctrl)
	ctrlModel.EXPECT().IsControllerModel().Return(true)

	ctrlState := mocks.NewMockState(ctrl)
	state1 := mocks.NewMockState(ctrl)
	ctrlState.EXPECT().Release().AnyTimes()
	ctrlState.EXPECT().Model().Return(ctrlModel, nil)
	state1.EXPECT().Release()

	s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil)
	var agentStream string
	assertions := []*gomock.Call{
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		// 1. Check controller model.
		// - check agent version;
		ctrlModel.EXPECT().AgentVersion().Return(version.MustParse("2.9.1"), nil),
		// - check mongo status;
		ctrlState.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
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
		// - check mongo version;
		s.statePool.EXPECT().MongoVersion().Return("4.4", nil),
		// - check if the model has win machines;
		ctrlState.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(nil, nil),
		// - check if the model has deprecated ubuntu machines;
		ctrlState.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(nil, nil),
		ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID.String()}, nil),

		// 2. Check other models.
		s.statePool.EXPECT().Get(model1ModelUUID.String()).Return(state1, nil),
		state1.EXPECT().Model().Return(model1, nil),
		// - check agent version;
		model1.EXPECT().AgentVersion().Return(version.MustParse("2.9.1"), nil),
		//  - check if model migration is ongoing;
		model1.EXPECT().MigrationMode().Return(state.MigrationModeNone),
		// - check if the model has win machines;
		state1.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(nil, nil),
		// - check if the model has deprecated ubuntu machines;
		state1.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(nil, nil),
	}
	if !dryRun {
		assertions = append(assertions,
			s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil),
			ctrlState.EXPECT().SetModelAgentVersion(version.MustParse("3.0.0"), &agentStream, false).Return(nil),
		)
	}
	gomock.InOrder(assertions...)

	err = api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:    ctrlModelTag.String(),
			ToVersion:   version.MustParse("3.0.0"),
			AgentStream: agentStream,
			DryRun:      dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelForControllerModelJuju3(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, false)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelForControllerModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, true)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelForControllerModelJuju3Failed(c *gc.C) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.2"),
	})

	ctrlModelTag := coretesting.ModelTag
	model1ModelUUID, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ctrlModel := mocks.NewMockModel(ctrl)
	model1 := mocks.NewMockModel(ctrl)
	ctrlModel.EXPECT().IsControllerModel().Return(true)

	ctrlState := mocks.NewMockState(ctrl)
	state1 := mocks.NewMockState(ctrl)
	ctrlState.EXPECT().Release().AnyTimes()
	ctrlState.EXPECT().Model().Return(ctrlModel, nil)
	state1.EXPECT().Release()

	s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil)

	gomock.InOrder(
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		// 1. Check controller model.
		// - check agent version;
		ctrlModel.EXPECT().AgentVersion().Return(version.MustParse("2.9.1"), nil),
		// - check mongo status;
		ctrlState.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
			Members: []replicaset.MemberStatus{
				{
					Id:      1,
					Address: "1.1.1.1",
					State:   replicaset.FatalState,
				},
				{
					Id:      2,
					Address: "2.2.2.2",
					State:   replicaset.ArbiterState,
				},
				{
					Id:      3,
					Address: "3.3.3.3",
					State:   replicaset.RecoveringState,
				},
			},
		}, nil),
		// - check mongo version;
		s.statePool.EXPECT().MongoVersion().Return("4.3", nil),
		// - check if the model has win machines;
		ctrlState.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(map[string]int{"win10": 1, "win7": 2}, nil),
		// - check if the model has deprecated ubuntu machines;
		ctrlState.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(map[string]int{"xenial": 2}, nil),
		ctrlModel.EXPECT().Owner().Return(names.NewUserTag("admin")),
		ctrlModel.EXPECT().Name().Return("controller"),

		ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID.String()}, nil),
		// 2. Check other models.
		s.statePool.EXPECT().Get(model1ModelUUID.String()).Return(state1, nil),
		state1.EXPECT().Model().Return(model1, nil),
		// - check agent version;
		model1.EXPECT().AgentVersion().Return(version.MustParse("2.9.0"), nil),
		//  - check if model migration is ongoing;
		model1.EXPECT().MigrationMode().Return(state.MigrationModeExporting),
		// - check if the model has win machines;
		state1.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(map[string]int{"win10": 1, "win7": 3}, nil),
		// - check if the model has deprecated ubuntu machines;
		state1.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(map[string]int{
			"artful": 1, "cosmic": 2, "disco": 3, "eoan": 4, "groovy": 5,
			"hirsute": 6, "impish": 7, "precise": 8, "quantal": 9, "raring": 10,
			"saucy": 11, "trusty": 12, "utopic": 13, "vivid": 14, "wily": 15,
			"xenial": 16, "yakkety": 17, "zesty": 18,
		}, nil),
		model1.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model1.EXPECT().Name().Return("model-1"),
	)

	err = api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:  ctrlModelTag.String(),
			ToVersion: version.MustParse("3.0.0"),
		},
	)
	c.Assert(err.Error(), gc.Equals, `
cannot upgrade to "3.0.0" due to issues with these models:
"admin/controller":
- current model ("2.9.1") has to be upgraded to "2.9.2" at least
- unable to upgrade, database node 1 (1.1.1.1) has state FATAL, node 2 (2.2.2.2) has state ARBITER, node 3 (3.3.3.3) has state RECOVERING
- mongo version has to be "4.4" at least, but current version is "4.3"
- the model hosts deprecated windows machine(s): win10(1) win7(2)
- the model hosts deprecated ubuntu machine(s): xenial(2)
"admin/model-1":
- current model ("2.9.0") has to be upgraded to "2.9.2" at least
- model is under "exporting" mode, upgrade blocked
- the model hosts deprecated windows machine(s): win10(1) win7(3)
- the model hosts deprecated ubuntu machine(s): artful(1) cosmic(2) disco(3) eoan(4) groovy(5) hirsute(6) impish(7) precise(8) quantal(9) raring(10) saucy(11) trusty(12) utopic(13) vivid(14) wily(15) xenial(16) yakkety(17) zesty(18)`[1:])
}

func (s *modelManagerUpgradeSuite) assertUpgradeModelJuju3(c *gc.C, dryRun bool) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	modelUUID := coretesting.ModelTag.Id()
	model := mocks.NewMockModel(ctrl)
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release().AnyTimes()

	var agentStream string
	assertions := []*gomock.Call{
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(st, nil),
		st.EXPECT().Model().Return(model, nil),
		model.EXPECT().IsControllerModel().Return(false),

		// - check no upgrade series in process.
		st.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),

		// - check if the model has win machines;
		st.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(nil, nil),
		// - check if the model has deprecated ubuntu machines;
		st.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(nil, nil),
	}
	if !dryRun {
		assertions = append(assertions,
			s.statePool.EXPECT().Get(modelUUID).Return(st, nil),
			st.EXPECT().SetModelAgentVersion(version.MustParse("3.0.0"), &agentStream, false).Return(nil),
		)
	}
	gomock.InOrder(assertions...)

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:    coretesting.ModelTag.String(),
			ToVersion:   version.MustParse("3.0.0"),
			AgentStream: agentStream,
			DryRun:      dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelJuju3(c *gc.C) {
	s.assertUpgradeModelJuju3(c, false)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelJuju3(c, true)
}

func (s *modelManagerUpgradeSuite) TestUpgradeModelJuju3Failed(c *gc.C) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	modelUUID := coretesting.ModelTag.Id()
	model := mocks.NewMockModel(ctrl)
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release()

	gomock.InOrder(
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(st, nil),
		st.EXPECT().Model().Return(model, nil),
		model.EXPECT().IsControllerModel().Return(false),

		// - check no upgrade series in process.
		st.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),

		// - check if the model has win machines;
		st.EXPECT().MachineCountForSeries(
			"win2008r2", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016hv", "win2019", "win7", "win8", "win81", "win10",
		).Return(map[string]int{"win10": 1, "win7": 3}, nil),
		// - check if the model has deprecated ubuntu machines;
		st.EXPECT().MachineCountForSeries(
			"artful",
			"bionic",
			"cosmic",
			"disco",
			"eoan",
			"groovy",
			"hirsute",
			"impish",
			"precise",
			"quantal",
			"raring",
			"saucy",
			"trusty",
			"utopic",
			"vivid",
			"wily",
			"xenial",
			"yakkety",
			"zesty",
		).Return(map[string]int{
			"artful": 1, "cosmic": 2, "disco": 3, "eoan": 4, "groovy": 5,
			"hirsute": 6, "impish": 7, "precise": 8, "quantal": 9, "raring": 10,
			"saucy": 11, "trusty": 12, "utopic": 13, "vivid": 14, "wily": 15,
			"xenial": 16, "yakkety": 17, "zesty": 18,
		}, nil),
		model.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model.EXPECT().Name().Return("model-1"),
	)
	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:  coretesting.ModelTag.String(),
			ToVersion: version.MustParse("3.0.0"),
		},
	)
	c.Logf(err.Error())
	c.Assert(err.Error(), gc.Equals, `
cannot upgrade to "3.0.0" due to issues with these models:
"admin/model-1":
- unexpected upgrade series lock found
- the model hosts deprecated windows machine(s): win10(1) win7(3)
- the model hosts deprecated ubuntu machine(s): artful(1) cosmic(2) disco(3) eoan(4) groovy(5) hirsute(6) impish(7) precise(8) quantal(9) raring(10) saucy(11) trusty(12) utopic(13) vivid(14) wily(15) xenial(16) yakkety(17) zesty(18)`[1:])
}

func (s *modelManagerUpgradeSuite) TestAbortCurrentUpgrade(c *gc.C) {
	ctrl, api := s.getModelUpgraderAPI(c)
	defer ctrl.Finish()

	modelUUID := coretesting.ModelTag.Id()
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release()

	gomock.InOrder(
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(st, nil),
		st.EXPECT().AbortCurrentUpgrade().Return(nil),
	)
	err := api.AbortModelUpgrade(params.ModelParam{ModelTag: coretesting.ModelTag.String()})
	c.Assert(err, jc.ErrorIsNil)
}
