// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"fmt"

	"github.com/golang/mock/gomock"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type ValidateModelUpgradesSuite struct {
	jujutesting.IsolationSuite

	st          *mockState
	adminUser   names.UserTag
	authoriser  apiservertesting.FakeAuthorizer
	callContext context.ProviderCallContext
}

var _ = gc.Suite(&ValidateModelUpgradesSuite{})

func (s *ValidateModelUpgradesSuite) SetUpTest(c *gc.C) {
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

// TestValidateModelUpgradesWithNoModelTags tests that we don't fail if we don't
// pass any model tags.
func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesWithNoModelTags(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesWithInvalidModelTag(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: "!!!",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `"!!!" is not a valid tag`)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesWithModelWithNoPermission(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	statePool := mocks.NewMockStatePool(ctrl)
	authoriser := apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("user"),
	}

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `permission denied`)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForControllerModels(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(true)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	gomock.InOrder(
		statePool.EXPECT().MongoVersion().Return("4.4", nil),
		state.EXPECT().AllModelUUIDs().Return([]string{s.st.model.tag.Id()}, nil),
		statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil),
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), jc.ErrorIsNil)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForControllerModelsFailedMongoTooOld(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(true)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	gomock.InOrder(
		statePool.EXPECT().MongoVersion().Return("4.3", nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `mongo version is not equal or greater than "4.4"`)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForControllerModelsFailedHostsWinMachines(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(true)

	controllerState := mocks.NewMockState(ctrl)
	controllerState.EXPECT().Release()
	controllerState.EXPECT().Model().Return(model, nil)
	modelState := mocks.NewMockState(ctrl)
	modelID := coretesting.ModelTag.Id()

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(controllerState, nil)

	gomock.InOrder(
		statePool.EXPECT().MongoVersion().Return("4.4", nil),
		controllerState.EXPECT().AllModelUUIDs().Return([]string{s.st.model.tag.Id(), modelID}, nil),

		statePool.EXPECT().Get(s.st.model.tag.Id()).Return(controllerState, nil),
		controllerState.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),

		statePool.EXPECT().Get(modelID).Return(modelState, nil),
		modelState.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(1, nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, fmt.Sprintf("model %q hosts 1 windows machine\\(s\\)", modelID))
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForNonControllerModels(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	gomock.InOrder(
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), jc.ErrorIsNil)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForNonControllerModelsFailedHostsWinMachines(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(false, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	gomock.InOrder(
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(3, nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, fmt.Sprintf("model %q hosts 3 windows machine\\(s\\)", s.st.model.tag.Id()))
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForUpgradingMachines(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), gc.ErrorMatches, `unexpected upgrade series lock found`)
}

func (s *ValidateModelUpgradesSuite) TestValidateModelUpgradesForUpgradingMachinesWithForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	model := mocks.NewMockModel(ctrl)
	model.EXPECT().IsControllerModel().Return(false)

	state := mocks.NewMockState(ctrl)
	state.EXPECT().Release()
	state.EXPECT().Model().Return(model, nil)
	state.EXPECT().HasUpgradeSeriesLocks().Return(true, nil)

	statePool := mocks.NewMockStatePool(ctrl)
	statePool.EXPECT().Get(s.st.model.tag.Id()).Return(state, nil)

	gomock.InOrder(
		state.EXPECT().MachineCountForSeries(
			"win10", "win2008r2", "win2012", "win2012", "win2012hv", "win2012hvr2", "win2012r2", "win2012r2",
			"win2016", "win2016", "win2016hv", "win2019", "win2019", "win7", "win8", "win81",
		).Return(0, nil),
	)

	api, err := modelmanager.NewModelManagerAPI(s.st, &mockState{}, statePool, nil, nil, s.authoriser, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.ValidateModelUpgrades(params.ValidateModelUpgradeParams{
		Models: []params.ValidateModelUpgradeParam{{
			ModelTag: s.st.model.tag.String(),
		}},
		Force: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.OneError(), jc.ErrorIsNil)
}

func (s *ValidateModelUpgradesSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
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
