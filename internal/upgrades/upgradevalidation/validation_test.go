// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"fmt"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
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
	c.Assert(err, gc.ErrorMatches, `server is unreachable`)
	c.Assert(blockers, gc.IsNil)
}

func (s *upgradeValidationSuite) TestModelUpgradeCheck(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blockers.String(), gc.Equals, `
"test-model":
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

	blocker, err := upgradevalidation.CheckForDeprecatedUbuntuSeriesForModel(st, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker.Error(), gc.Equals, `the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04`)
}

func (s *upgradeValidationSuite) TestGetCheckTargetVersionForControllerModel(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.ErrorMatches, `current model \("2.9.29"\) has to be upgraded to "2.9.30" at least`)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("3.0.0"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("1.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, gc.ErrorMatches, `downgrade is not allowed`)
	c.Assert(blocker, gc.IsNil)

	blocker, err = upgradevalidation.GetCheckTargetVersionForModel(
		semversion.MustParse("4.1.1"),
		upgradevalidation.UpgradeControllerAllowed,
	)(nil, agentService)
	c.Assert(err, gc.ErrorMatches, `upgrading controller to "4.1.1" is not supported from "2.9.31"`)
	c.Assert(blocker, gc.IsNil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)(nil, nil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(environscloudspec.CloudSpec{Type: "foo"})(nil, nil)
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

	blocker, err := upgradevalidation.GetCheckForLXDVersion(cloudSpec.CloudSpec)(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(blocker, gc.NotNil)
	c.Assert(blocker.Error(), gc.Equals, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}
