// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	"github.com/juju/names/v5"
	"github.com/juju/replicaset/v3"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/state"
)

func (s *upgradeValidationSuite) TestValidatorsForControllerUpgradeJuju3(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	ctrlModelTag := names.NewModelTag("deadpork-0bad-400d-8000-4b1d0d06f00d")
	model1ModelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)

	ctrlState := mocks.NewMockState(ctrl)
	ctrlModel := mocks.NewMockModel(ctrl)

	state1 := mocks.NewMockState(ctrl)
	model1 := mocks.NewMockModel(ctrl)

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}

	// 1. Check controller model.
	// - check agent version;
	ctrlModel.EXPECT().AgentVersion().Return(version.MustParse("3.666.1"), nil)
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
	}, nil)
	// - check mongo version;
	statePool.EXPECT().MongoVersion().Return("4.4", nil)
	ctrlState.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	ctrlState.EXPECT().AllMachinesCount().Return(0, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")
	// 2. Check hosted models.
	// - check agent version;
	model1.EXPECT().AgentVersion().Return(version.MustParse("2.9.1"), nil)
	//  - check if model migration is ongoing;
	model1.EXPECT().MigrationMode().Return(state.MigrationModeNone)
	state1.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	state1.EXPECT().AllMachinesCount().Return(0, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	targetVersion := version.MustParse("3.666.2")
	validators := upgradevalidation.ValidatorsForControllerModelUpgrade(targetVersion, cloudSpec.CloudSpec)
	checker := upgradevalidation.NewModelUpgradeCheck(ctrlModelTag.Id(), statePool, ctrlState, ctrlModel, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)

	validators = upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion, cloudSpec.CloudSpec)
	checker = upgradevalidation.NewModelUpgradeCheck(model1ModelTag.Id(), statePool, state1, model1, validators...)
	blockers, err = checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}

func (s *upgradeValidationSuite) TestValidatorsForModelUpgradeJuju3(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	modelTag := coretesting.ModelTag
	statePool := mocks.NewMockStatePool(ctrl)
	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)

	server := mocks.NewMockServer(ctrl)
	serverFactory := mocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)
	cloudSpec := lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}

	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	st.EXPECT().AllMachinesCount().Return(0, nil)

	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	targetVersion := version.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelUpgrade(false, targetVersion, cloudSpec.CloudSpec)
	checker := upgradevalidation.NewModelUpgradeCheck(modelTag.Id(), statePool, st, model, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}
