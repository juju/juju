// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands/mocks"
	"github.com/juju/juju/cmd/juju/config"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/jujuclient"
	coretesting "github.com/juju/juju/testing"
)

type UpgradeControllerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite

	modelUpgrader  *mocks.MockModelUpgraderAPI
	controllerAPI  *mocks.MockControllerAPI
	modelConfigAPI *mocks.MockModelConfigAPI
	store          *jujuclient.MemStore
}

var _ = gc.Suite(&UpgradeControllerSuite{})

func (s *UpgradeControllerSuite) upgradeControllerCommand(c *gc.C) (*gomock.Controller, cmd.Command) {
	ctrl := gomock.NewController(c)
	s.modelUpgrader = mocks.NewMockModelUpgraderAPI(ctrl)
	s.controllerAPI = mocks.NewMockControllerAPI(ctrl)
	s.modelConfigAPI = mocks.NewMockModelConfigAPI(ctrl)
	cmd := &upgradeControllerCommand{
		baseUpgradeCommand: baseUpgradeCommand{
			modelUpgraderAPI: s.modelUpgrader,
			controllerAPI:    s.controllerAPI,
			modelConfigAPI:   s.modelConfigAPI,
		},
	}
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "arthur"
	store.Controllers["arthur"] = jujuclient.ControllerDetails{}
	store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/controller",
		Models: map[string]jujuclient.ModelDetails{"admin/controller": {
			ModelType: model.IAAS,
			ModelUUID: coretesting.ModelTag.Id(),
		}},
	}
	store.Accounts["arthur"] = jujuclient.AccountDetails{
		User: "king",
	}
	s.store = store
	cmd.SetClientStore(s.store)
	return ctrl, modelcmd.WrapController(cmd)
}

func (s *UpgradeControllerSuite) TestUpgradeWrongPermissions(c *gc.C) {
	ctrl, com := s.upgradeControllerCommand(c)
	defer ctrl.Finish()

	details, err := s.store.AccountDetails("arthur")
	c.Assert(err, jc.ErrorIsNil)
	details.LastKnownAccess = string(permission.ReadAccess)
	err = s.store.UpdateAccount("arthur", *details)
	c.Assert(err, jc.ErrorIsNil)

	err = cmdtesting.InitCommand(com, []string{})
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	err = com.Run(ctx)
	expectedErrMsg := fmt.Sprintf("upgrade not possible missing"+
		" permissions, current level %q, need: %q", details.LastKnownAccess, permission.SuperuserAccess)
	c.Assert(err, gc.ErrorMatches, expectedErrMsg)
}

func (s *UpgradeControllerSuite) TestUpgradeDifferentUser(c *gc.C) {
	ctrl, com := s.upgradeControllerCommand(c)
	defer ctrl.Finish()

	s.modelUpgrader.EXPECT().Close().Return(nil)
	s.controllerAPI.EXPECT().Close().Return(nil)
	s.modelConfigAPI.EXPECT().Close().Return(nil)

	attrs := coretesting.CustomModelConfig(c, coretesting.FakeConfig()).AllAttrs()
	s.modelConfigAPI.EXPECT().ModelGet().Return(attrs, nil)
	s.controllerAPI.EXPECT().ModelConfig().Return(config.Attrs{"uuid": coretesting.ModelTag.Id()}, nil)
	s.modelUpgrader.EXPECT().UpgradeModel(coretesting.ModelTag.Id(), version.MustParse("0.0.0"), "proposed", false, false).Return(version.MustParse("6.6.6"), nil)

	err := s.store.UpdateAccount("arthur", jujuclient.AccountDetails{
		User:            "rick",
		LastKnownAccess: string(permission.SuperuserAccess),
		Password:        "dummy-secret",
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = cmdtesting.RunCommand(c, com, "--agent-stream", "proposed")
	c.Assert(err, jc.ErrorIsNil)
}
