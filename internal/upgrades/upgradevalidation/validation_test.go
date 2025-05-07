// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
)

var _ = tc.Suite(&upgradeValidationSuite{})

type upgradeValidationSuite struct {
	jujutesting.IsolationSuite
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

	st := mocks.NewMockState(ctrl)
	agentVersion := mocks.NewMockModelAgentService(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck(st, "test-model", agentVersion,
		func(st upgradevalidation.State, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
		func(st upgradevalidation.State, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
			return nil, errors.New("server is unreachable")
		},
	)

	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, tc.IsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	agentService := mocks.NewMockModelAgentService(ctrl)

	checker := upgradevalidation.NewModelUpgradeCheck(st, "test-model", agentService,
		func(st upgradevalidation.State, modelAgentService upgradevalidation.ModelAgentService) (*upgradevalidation.Blocker, error) {
			return upgradevalidation.NewBlocker("model migration is in process"), nil
		},
	)

	blockers, err := checker.Validate()
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

	st := mocks.NewMockState(ctrl)
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(map[string]int{"ubuntu@20.04": 1, "ubuntu@22.04": 1, "ubuntu@24.04": 2}, nil)
	st.EXPECT().AllMachinesCount().Return(5, nil)

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel(st, nil)
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
	gomock.InOrder(
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.29"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
		agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("2.9.31"), nil),
	)

	blocker, err := upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, tc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, tc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, tc.ErrorMatches, `upgrading controller to "4.1.1" is not supported from "2.9.31"`)
	c.Assert(blocker, tc.IsNil)
}
