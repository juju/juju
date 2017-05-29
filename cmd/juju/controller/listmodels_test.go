// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
)

type ModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeModelMgrAPIClient
	store *jujuclient.MemStore
}

var _ = gc.Suite(&ModelsSuite{})

type fakeModelMgrAPIClient struct {
	err          error
	user         string
	models       []base.UserModel
	all          bool
	inclMachines bool
	denyAccess   bool
	infos        []params.ModelInfoResult
}

func (f *fakeModelMgrAPIClient) Close() error {
	return nil
}

func (f *fakeModelMgrAPIClient) ListModels(user string) ([]base.UserModel, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.user = user
	return f.models, nil
}

func (f *fakeModelMgrAPIClient) AllModels() ([]base.UserModel, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.all = true
	return f.models, nil
}

func (f *fakeModelMgrAPIClient) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	if f.infos != nil {
		return f.infos, nil
	}
	agentVersion, _ := version.Parse("2.2-rc1")
	results := make([]params.ModelInfoResult, len(tags))
	for i, tag := range tags {
		for _, model := range f.models {
			if model.UUID != tag.Id() {
				continue
			}
			result := &params.ModelInfo{
				Name:         model.Name,
				UUID:         model.UUID,
				OwnerTag:     names.NewUserTag(model.Owner).String(),
				CloudTag:     "cloud-dummy",
				Status:       params.EntityStatus{},
				AgentVersion: &agentVersion,
			}
			switch model.Name {
			case "test-model1":
				last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
				result.Status.Status = status.Active
				if f.user != "" {
					result.Users = []params.ModelUserInfo{{
						UserName:       f.user,
						LastConnection: &last1,
						Access:         params.ModelReadAccess,
					}}
				}
				if f.inclMachines {
					one := uint64(1)
					result.Machines = []params.ModelMachineInfo{
						{Id: "0", Hardware: &params.MachineHardware{Cores: &one}}, {Id: "1"},
					}
				}
			case "test-model2":
				last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)
				result.Status.Status = status.Active
				if f.user != "" {
					result.Users = []params.ModelUserInfo{{
						UserName:       f.user,
						LastConnection: &last2,
						Access:         params.ModelWriteAccess,
					}}
				}
			case "test-model3":
				if f.denyAccess {
					results[i].Error = &params.Error{
						Message: "permission denied",
						Code:    params.CodeUnauthorized,
					}
				}
				result.Status.Status = status.Destroying
			}
			results[i].Result = result
		}
	}
	return results, nil
}

func (s *ModelsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	models := []base.UserModel{
		{
			Name:  "test-model1",
			Owner: "admin",
			UUID:  "test-model1-UUID",
		}, {
			Name:  "test-model2",
			Owner: "carlotta",
			UUID:  "test-model2-UUID",
		}, {
			Name:  "test-model3",
			Owner: "daiwik@external",
			UUID:  "test-model3-UUID",
		},
	}
	s.api = &fakeModelMgrAPIClient{
		models: models,
		user:   "admin",
	}
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "fake"
	s.store.Controllers["fake"] = jujuclient.ControllerDetails{}
	s.store.Models["fake"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/test-model1",
	}
	s.store.Accounts["fake"] = jujuclient.AccountDetails{
		User:     "admin",
		Password: "password",
	}
}

func (s *ModelsSuite) newCommand() cmd.Command {
	return controller.NewListModelsCommandForTest(s.api, s.api, s.store)
}

func (s *ModelsSuite) TestModelsOwner(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"test-model1*                 dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestModelsNonOwner(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--user", "bob")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "bob")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"admin/test-model1*           dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestAllModels(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.all, jc.IsTrue)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"admin/test-model1*           dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestAllModelsNoneCurrent(c *gc.C) {
	delete(s.store.Models, "fake")
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"test-model1                  dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestModelsUUID(c *gc.C) {
	s.api.inclMachines = true
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        UUID              Cloud/Region  Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 test-model1-UUID  dummy         active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         test-model2-UUID  dummy         active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  test-model3-UUID  dummy         destroying         0      -  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestModelsMachineInfo(c *gc.C) {
	s.api.inclMachines = true
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.user, gc.Equals, "admin")
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 dummy         active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying         0      -  -       never connected\n"+
		"\n")
}

func (s *ModelsSuite) TestAllModelsWithOneUnauthorised(c *gc.C) {
	s.api.denyAccess = true
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                 Cloud/Region  Status  Access  Last connection\n"+
		"test-model1*          dummy         active  read    2015-03-20\n"+
		"carlotta/test-model2  dummy         active  write   2015-03-01\n"+
		"\n")
}

func (s *ModelsSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *ModelsSuite) TestModelsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, gc.ErrorMatches, "cannot list models: permission denied")
}

func createBasicModelInfo() *params.ModelInfo {
	agentVersion, _ := version.Parse("2.2-rc1")
	return &params.ModelInfo{
		Name:           "basic-model",
		UUID:           testing.ModelTag.Id(),
		ControllerUUID: testing.ControllerTag.Id(),
		OwnerTag:       names.NewUserTag("owner").String(),
		Life:           params.Dead,
		CloudTag:       names.NewCloudTag("altostratus").String(),
		CloudRegion:    "mid-level",
		AgentVersion:   &agentVersion,
	}
}

func (s *ModelsSuite) TestWithIncompleteModels(c *gc.C) {
	basicAndStatusInfo := createBasicModelInfo()
	basicAndStatusInfo.Status = params.EntityStatus{
		Status: status.Busy,
	}

	basicAndUsersInfo := createBasicModelInfo()
	basicAndUsersInfo.Users = []params.ModelUserInfo{
		params.ModelUserInfo{"admin", "display name", nil, params.UserAccessPermission("admin")},
	}

	basicAndMachinesInfo := createBasicModelInfo()
	basicAndMachinesInfo.Machines = []params.ModelMachineInfo{
		params.ModelMachineInfo{Id: "2"},
		params.ModelMachineInfo{Id: "12"},
	}

	s.api.infos = []params.ModelInfoResult{
		params.ModelInfoResult{Result: createBasicModelInfo()},
		params.ModelInfoResult{Result: basicAndStatusInfo},
		params.ModelInfoResult{Result: basicAndUsersInfo},
		params.ModelInfoResult{Result: basicAndMachinesInfo},
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
Controller: fake

Model              Cloud/Region           Status  Machines  Cores  Access  Last connection
owner/basic-model  altostratus/mid-level  -              0      -  -       never connected
owner/basic-model  altostratus/mid-level  busy           0      -  -       never connected
owner/basic-model  altostratus/mid-level  -              0      -  admin   never connected
owner/basic-model  altostratus/mid-level  -              2      -  -       never connected

`[1:])
}

func (s *ModelsSuite) assertAgentVersionPresent(c *gc.C, testInfo *params.ModelInfo, checker gc.Checker) {
	s.api.infos = []params.ModelInfoResult{
		params.ModelInfoResult{Result: testInfo},
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), checker, "agent-version")
}

func (s *ModelsSuite) TestListModelsWithAgent(c *gc.C) {
	basicInfo := createBasicModelInfo()
	s.assertAgentVersionPresent(c, basicInfo, jc.Contains)
}

func (s *ModelsSuite) TestListModelsWithNoAgent(c *gc.C) {
	basicInfo := createBasicModelInfo()
	basicInfo.AgentVersion = nil
	s.assertAgentVersionPresent(c, basicInfo, gc.Not(jc.Contains))
}
