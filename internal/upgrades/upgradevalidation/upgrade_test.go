// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/semversion"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/provider/lxd"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
)

func (s *upgradeValidationSuite) TestValidatorsForControllerUpgradeJuju3(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)

	validators = upgradevalidation.ModelValidatorsForControllerModelUpgrade(targetVersion)
	checker = upgradevalidation.NewModelUpgradeCheck(state1, "test-model", agentVersion, validators...)
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

	//modelTag := coretesting.ModelTag
	st := mocks.NewMockState(ctrl)
	agentService := mocks.NewMockModelAgentService(ctrl)

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

	targetVersion := semversion.MustParse("3.0.0")
	validators := upgradevalidation.ValidatorsForModelUpgrade(false, targetVersion)
	checker := upgradevalidation.NewModelUpgradeCheck(st, "test-model", agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers, gc.IsNil)
}
