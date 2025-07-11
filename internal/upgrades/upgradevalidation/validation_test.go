// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
)

func TestUpgradeValidationSuite(t *testing.T) {
	tc.Run(t, &upgradeValidationSuite{})
}

type upgradeValidationSuite struct {
	testhelpers.IsolationSuite
}

func (s *upgradeValidationSuite) TestModelUpgradeBlockers(c *tc.C) {
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
	c.Assert(blockers1.String(), tc.Equals, `
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

func (s *upgradeValidationSuite) TestModelUpgradeCheckFailEarly(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	agentVersion := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentVersion,
		MachineService:    machineService,
	}

	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices,
		func(context.Context, upgradevalidation.ValidatorServices) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(context.Context, upgradevalidation.ValidatorServices) (*upgradevalidation.Blocker, error) {
			return nil, errors.New("server is unreachable")
		},
	)

	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, tc.IsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	agentVersion := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentVersion,
		MachineService:    machineService,
	}

	checker := upgradevalidation.NewModelUpgradeCheck("test-model", validatorServices,
		func(context.Context, upgradevalidation.ValidatorServices) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
	)

	blockers, err := checker.Validate(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers.String(), tc.Equals, `
"test-model":
- model migration is in process`[1:])
}

func (s *upgradeValidationSuite) TestCheckForDeprecatedUbuntuSeriesForModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	agentVersion := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentVersion,
		MachineService:    machineService,
	}
	machineNames := []machine.Name{"0", "1", "2"}
	machineService.EXPECT().AllMachineNames(gomock.Any()).Return(machineNames, nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("0")).Return(base.MustParseBaseFromString("ubuntu@18.04"), nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("1")).Return(base.MustParseBaseFromString("ubuntu@22.04"), nil)
	machineService.EXPECT().GetMachineBase(gomock.Any(), machine.Name("2")).Return(base.MustParseBaseFromString("ubuntu@20.04"), nil)

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel(c.Context(), validatorServices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker.Error(), tc.Equals, `the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForControllerModel(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]semversion.Number{
		3: semversion.MustParse("2.9.30"),
	})

	agentService := mocks.NewMockModelAgentService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)
	validatorServices := upgradevalidation.ValidatorServices{
		ModelAgentService: agentService,
		MachineService:    machineService,
	}
	machineService.EXPECT().AllMachineNames(gomock.Any()).Return(nil, nil).AnyTimes()
	gomock.InOrder(
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.29"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(c.Context(), validatorServices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(c.Context(), validatorServices)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(c.Context(), validatorServices)
	c.Assert(err, tc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(c.Context(), validatorServices)
	c.Assert(err, tc.ErrorMatches, `upgrading controller to "4.1.1" is not supported from "2.9.31"`)
	c.Assert(blocker, tc.IsNil)
}
