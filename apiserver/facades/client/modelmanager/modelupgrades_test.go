// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/replicaset/v2"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades/upgradevalidation"
	jujuversion "github.com/juju/juju/version"
)

type modelManagerSuite struct {
	jujutesting.IsolationSuite

	st          *mockState
	adminUser   names.UserTag
	authoriser  apiservertesting.FakeAuthorizer
	callContext context.ProviderCallContext

	statePool        *mocks.MockStatePool
	toolsFinder      *mocks.MockToolsFinder
	bootstrapEnviron *mocks.MockBootstrapEnviron
	blockChecker     *mocks.MockBlockCheckerInterface
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.st = &mockState{
		model: s.createModel(c, s.adminUser),
	}
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *modelManagerSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
	attrs := dummy.SampleConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return &mockModel{
		tag:                 names.NewModelTag(utils.MustNewUUID().String()),
		owner:               user,
		cfg:                 cfg,
		setCloudCredentialF: func(tag names.CloudCredentialTag) (bool, error) { return false, nil },
	}
}

func (s *modelManagerSuite) getModelManagerAPI(c *gc.C) (*gomock.Controller, *modelmanager.ModelManagerAPI) {
	ctrl := gomock.NewController(c)
	s.statePool = mocks.NewMockStatePool(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.bootstrapEnviron = mocks.NewMockBootstrapEnviron(ctrl)
	s.blockChecker = mocks.NewMockBlockCheckerInterface(ctrl)

	api, err := modelmanager.NewModelManagerAPI(
		s.st, &mockState{}, s.statePool,
		s.toolsFinder,
		func() (environs.BootstrapEnviron, error) {
			return s.bootstrapEnviron, nil
		},
		nil, s.blockChecker, s.authoriser, s.st.model, s.callContext,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl, api
}

// TestValidateModelUpgradesWithNoModelTags tests that we don't fail if we don't
// pass any model tags.
func (s *modelManagerSuite) TestValidateModelUpgradesV9WithNoModelTags(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	v9 := modelmanager.ModelManagerAPIV9{api}
	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *modelManagerSuite) TestValidateModelUpgradesV9WithInvalidModelTag(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	v9 := modelmanager.ModelManagerAPIV9{api}
	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: "!!!",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `"!!!" is not a valid tag`)
}

func (s *modelManagerSuite) TestValidateModelUpgradesV9WithModelWithNoPermission(c *gc.C) {
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("user"),
	}

	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()
	v9 := modelmanager.ModelManagerAPIV9{api}

	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestValidateModelUpgradesV9ForUpgradingMachines(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil)

	s.statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	v9 := modelmanager.ModelManagerAPIV9{api}
	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `unexpected upgrade series lock found`)
}

func (s *modelManagerSuite) TestValidateModelUpgradesV9ForUpgradingMachinesWithForce(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()
	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil)

	s.statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	v9 := modelmanager.ModelManagerAPIV9{api}
	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
		Force: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestValidateModelUpgradesV9ForControllerModels(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(nil)

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(true)
	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	s.statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	v9 := modelmanager.ModelManagerAPIV9{api}
	results, err := v9.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestUpgradeModelWithInvalidModelTag(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	err := api.UpgradeModel(params.UpgradeModel{ModelTag: "!!!"})
	c.Assert(err, gc.ErrorMatches, `"!!!" is not a valid tag`)
}

func (s *modelManagerSuite) TestUpgradeModelWithModelWithNoPermission(c *gc.C) {
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("user"),
	}
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag: s.st.model.tag.String(),
			Version:  version.MustParse("3.0-beta1"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestUpgradeModelWithChangeNotAllowed(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.blockChecker.EXPECT().ChangeAllowed().Return(errors.Errorf("the operation has been blocked"))

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag: s.st.model.tag.String(),
			Version:  version.MustParse("3.0-beta1"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `the operation has been blocked`)
}

func (s *modelManagerSuite) assertUpgradeModelForControllerModelJuju3(c *gc.C, dryRun bool) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	ctrlModelTag := s.st.model.tag
	model1ModelUUID := coretesting.ModelTag.Id()
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
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
		ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID}, nil),

		// 2. Check other models.
		s.statePool.EXPECT().Get(model1ModelUUID).Return(state1, nil),
		state1.EXPECT().Model().Return(model1, nil),
		// - check agent version;
		model1.EXPECT().AgentVersion().Return(version.MustParse("2.9.1"), nil),
		//  - check if model migration is ongoing;
		model1.EXPECT().MigrationMode().Return(state.MigrationModeNone),
		// - check if the model has win machines;
		state1.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	}
	if !dryRun {
		assertions = append(assertions,
			s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil),
			ctrlState.EXPECT().SetModelAgentVersion(version.MustParse("3.0-beta1"), &agentStream, false).Return(nil),
		)
	}
	gomock.InOrder(assertions...)

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:    ctrlModelTag.String(),
			Version:     version.MustParse("3.0-beta1"),
			AgentStream: agentStream,
			DryRun:      dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestUpgradeModelForControllerModelJuju3(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, false)
}

func (s *modelManagerSuite) TestUpgradeModelForControllerModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, true)
}

func (s *modelManagerSuite) TestUpgradeModelForControllerModelJuju3Failed(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.2"),
	})

	ctrlModelTag := s.st.model.tag
	model1ModelUUID := coretesting.ModelTag.Id()
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
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(8, nil),
		ctrlModel.EXPECT().Owner().Return(names.NewUserTag("admin")),
		ctrlModel.EXPECT().Name().Return("controller"),

		ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID}, nil),
		// 2. Check other models.
		s.statePool.EXPECT().Get(model1ModelUUID).Return(state1, nil),
		state1.EXPECT().Model().Return(model1, nil),
		// - check agent version;
		model1.EXPECT().AgentVersion().Return(version.MustParse("2.9.0"), nil),
		//  - check if model migration is ongoing;
		model1.EXPECT().MigrationMode().Return(state.MigrationModeExporting),
		// - check if the model has win machines;
		state1.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(6, nil),
		model1.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model1.EXPECT().Name().Return("model-1"),
	)

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag: ctrlModelTag.String(),
			Version:  version.MustParse("3.0-beta1"),
		},
	)
	c.Assert(err.Error(), gc.Equals, `
cannot upgrade to "3.0-beta1" due to issues with these models:
"admin/controller":
- current model ("2.9.1") has to be upgraded to "2.9.2" at least
- unable to upgrade, database node 1 (1.1.1.1) has state FATAL, node 2 (2.2.2.2) has state ARBITER, node 3 (3.3.3.3) has state RECOVERING
- mongo version has to be "4.4" at least, but current version is "4.3"
- windows is not supported but the model hosts 8 windows machine(s)
"admin/model-1":
- current model ("2.9.0") has to be upgraded to "2.9.2" at least
- model is under "exporting" mode, upgrade blocked
- windows is not supported but the model hosts 6 windows machine(s)`[1:])
}

func (s *modelManagerSuite) assertUpgradeModelJuju3(c *gc.C, dryRun bool) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	modelUUID := s.st.model.tag.Id()
	model := mocks.NewMockModel(ctrl)
	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release().AnyTimes()

	var agentStream string
	assertions := []*gomock.Call{
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(state, nil),
		state.EXPECT().Model().Return(model, nil),
		model.EXPECT().IsControllerModel().Return(false),

		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil),

		// - check if the model has win machines;
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	}
	if !dryRun {
		assertions = append(assertions,
			s.statePool.EXPECT().Get(modelUUID).Return(state, nil),
			state.EXPECT().SetModelAgentVersion(version.MustParse("3.0-beta1"), &agentStream, false).Return(nil),
		)
	}
	gomock.InOrder(assertions...)

	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag:    s.st.model.tag.String(),
			Version:     version.MustParse("3.0-beta1"),
			AgentStream: agentStream,
			DryRun:      dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestUpgradeModelJuju3(c *gc.C) {
	s.assertUpgradeModelJuju3(c, false)
}

func (s *modelManagerSuite) TestUpgradeModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelJuju3(c, true)
}

func (s *modelManagerSuite) TestUpgradeModelJuju3Failed(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinMajorUpgradeVersion, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	modelUUID := s.st.model.tag.Id()
	model := mocks.NewMockModel(ctrl)
	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()

	gomock.InOrder(
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(state, nil),
		state.EXPECT().Model().Return(model, nil),
		model.EXPECT().IsControllerModel().Return(false),

		// - check no upgrade series in process.
		state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil),

		// - check if the model has win machines;
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(10, nil),
		model.EXPECT().Owner().Return(names.NewUserTag("admin")),
		model.EXPECT().Name().Return("model-1"),
	)
	err := api.UpgradeModel(
		params.UpgradeModel{
			ModelTag: s.st.model.tag.String(),
			Version:  version.MustParse("3.0-beta1"),
		},
	)
	c.Logf(err.Error())
	c.Assert(err.Error(), gc.Equals, `
cannot upgrade to "3.0-beta1" due to issues with these models:
"admin/model-1":
- unexpected upgrade series lock found
- windows is not supported but the model hosts 10 windows machine(s)`[1:])
}

func (s *modelManagerSuite) TestAbortCurrentUpgrade(c *gc.C) {
	ctrl, api := s.getModelManagerAPI(c)
	defer ctrl.Finish()

	modelUUID := s.st.model.tag.Id()
	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()

	gomock.InOrder(
		s.blockChecker.EXPECT().ChangeAllowed().Return(nil),
		s.statePool.EXPECT().Get(modelUUID).Return(state, nil),
		state.EXPECT().AbortCurrentUpgrade().Return(nil),
	)
	err := api.AbortCurrentUpgrade()
	c.Assert(err, jc.ErrorIsNil)
}
