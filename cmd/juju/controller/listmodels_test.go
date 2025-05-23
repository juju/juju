// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"context"
	"regexp"
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

type ModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeModelMgrAPIClient
	store *jujuclient.MemStore
}

func TestModelsSuite(t *stdtesting.T) {
	tc.Run(t, &ModelsSuite{})
}

type fakeModelMgrAPIClient struct {
	*testhelpers.Stub

	err   error
	infos []params.ModelInfoResult
	units map[string]int
}

func (f *fakeModelMgrAPIClient) Close() error {
	f.MethodCall(f, "Close")
	return nil
}

func (f *fakeModelMgrAPIClient) ListModels(ctx context.Context, user string) ([]base.UserModel, error) {
	f.MethodCall(f, "ListModels", user)
	if f.err != nil {
		return nil, f.err
	}
	return f.convertInfosToUserModels(), nil
}

func (f *fakeModelMgrAPIClient) AllModels(ctx context.Context) ([]base.UserModel, error) {
	f.MethodCall(f, "AllModels")
	if f.err != nil {
		return nil, f.err
	}
	return f.convertInfosToUserModels(), nil
}

func (f *fakeModelMgrAPIClient) ListModelSummaries(ctx context.Context, user string, all bool) ([]base.UserModelSummary, error) {
	f.MethodCall(f, "ListModelSummaries", user, all)
	if f.err != nil {
		return nil, f.err
	}
	results := make([]base.UserModelSummary, len(f.infos))
	for i, info := range f.infos {
		results[i] = base.UserModelSummary{}
		if info.Error != nil {
			results[i].Error = info.Error
			continue
		}
		cloud, err := names.ParseCloudTag(info.Result.CloudTag)
		if err != nil {
			cloud = names.NewCloudTag("aws")
		}
		cred, err := names.ParseCloudCredentialTag(info.Result.CloudCredentialTag)
		if err != nil {
			cred = names.NewCloudCredentialTag("foo/bob/one")
		}
		results[i] = base.UserModelSummary{
			Name:            info.Result.Name,
			Type:            model.ModelType(info.Result.Type),
			UUID:            info.Result.UUID,
			ControllerUUID:  info.Result.ControllerUUID,
			IsController:    info.Result.IsController,
			ProviderType:    info.Result.ProviderType,
			Cloud:           cloud.Id(),
			CloudRegion:     info.Result.CloudRegion,
			CloudCredential: cred.Id(),
			Namespace:       info.Result.Namespace,
			Life:            info.Result.Life,
			Status: base.Status{
				Status: info.Result.Status.Status,
				Info:   info.Result.Status.Info,
				Data:   make(map[string]interface{}),
				Since:  info.Result.Status.Since,
			},
			AgentVersion: info.Result.AgentVersion,
		}
		if info.Result.Migration != nil {
			migration := info.Result.Migration
			results[i].Migration = &base.MigrationSummary{
				Status:    migration.Status,
				StartTime: migration.Start,
				EndTime:   migration.End,
			}
		}
		if len(info.Result.Users) > 0 {
			for _, u := range info.Result.Users {
				if u.UserName == user {
					results[i].ModelUserAccess = string(u.Access)
					results[i].UserLastConnection = u.LastConnection
					break
				}
			}
		}
		if len(info.Result.Machines) > 0 {
			results[i].Counts = []base.EntityCount{
				{string(params.Machines), int64(len(info.Result.Machines))},
			}
			cores := uint64(0)
			for _, machine := range info.Result.Machines {
				if machine.Hardware != nil && machine.Hardware.Cores != nil {
					cores += *machine.Hardware.Cores
				}
			}
			if cores > 0 {
				results[i].Counts = append(results[i].Counts, base.EntityCount{string(params.Cores), int64(cores)})
			}
		}
		if count, ok := f.units[info.Result.Name]; ok && count > 0 {
			results[i].Counts = append(results[i].Counts, base.EntityCount{string(params.Units), int64(count)})
		}
	}
	return results, nil
}

func (f *fakeModelMgrAPIClient) ModelInfo(ctx context.Context, tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	f.MethodCall(f, "ModelInfo", tags)
	if f.infos != nil {
		return f.infos, nil
	}
	results := make([]params.ModelInfoResult, len(tags))
	for i, tag := range tags {
		for _, model := range f.infos {
			if model.Error == nil {
				if model.Result.UUID != tag.Id() {
					continue
				}
				results[i] = model
			}
		}
	}
	return results, nil
}

func (f *fakeModelMgrAPIClient) convertInfosToUserModels() []base.UserModel {
	models := make([]base.UserModel, len(f.infos))
	for i, info := range f.infos {
		if info.Error == nil {
			models[i] = base.UserModel{UUID: info.Result.UUID, Type: "local"}
		}
	}
	return models
}

func (s *ModelsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	models := []base.UserModel{
		{
			Name:      "test-model1",
			Namespace: "admin",
			UUID:      "test-model1-UUID",
			Type:      model.IAAS,
		}, {
			Name:      "test-model2",
			Namespace: "carlotta",
			UUID:      "test-model2-UUID",
			Type:      model.CAAS,
		}, {
			Name:      "test-model3",
			Namespace: "daiwik@external",
			UUID:      "test-model3-UUID",
			Type:      model.IAAS,
		},
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

	s.api = &fakeModelMgrAPIClient{
		Stub: &testhelpers.Stub{},
	}
	s.api.infos = convert(models)

	// Make api results interesting...
	// 1st model
	firstModel := s.api.infos[0].Result
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)
	firstModel.Users = []params.ModelUserInfo{{
		UserName:       "admin",
		LastConnection: &last1,
		Access:         params.ModelReadAccess,
	}}
	// 2nd model
	secondModel := s.api.infos[1].Result
	last2 := time.Date(2015, 3, 1, 0, 0, 0, 0, time.UTC)
	secondModel.Users = []params.ModelUserInfo{{
		UserName:       "admin",
		LastConnection: &last2,
		Access:         params.ModelWriteAccess,
	}}
	// 3rd model
	s.api.infos[2].Result.Status.Status = status.Destroying
}

func (s *ModelsSuite) TestModelsOwner(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Type   Status      Access  Last connection\n"+
		"test-model1*                 dummy         local  active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         local  active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         local  destroying  -       never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

// TestModelsForAdmin tests that a model admin user will get model credential.
// Credential will only appear in non-tabular format - either yaml or json.
func (s *ModelsSuite) TestModelsWithCredentials(c *tc.C) {
	for i, infoResult := range s.api.infos {
		// let's say only some models will have credentials returned from api...
		if i%2 == 0 {
			infoResult.Result.CloudCredentialTag = "cloudcred-some-cloud_some-owner_some-credential"
		}
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format=yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Contains, "credential")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsNonOwner(c *tc.C) {
	// Ensure fake api caters to user 'bob'
	for _, apiInfo := range s.api.infos {
		if apiInfo.Error == nil {
			bobs := make([]params.ModelUserInfo, len(apiInfo.Result.Users))
			for i, u := range apiInfo.Result.Users {
				u.UserName = "bob"
				bobs[i] = u
			}
			apiInfo.Result.Users = bobs
		}
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--user", "bob")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Type   Status      Access  Last connection\n"+
		"admin/test-model1*           dummy         local  active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         local  active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         local  destroying  -       never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsNoneCurrent(c *tc.C) {
	delete(s.store.Models, "fake")
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Type   Status      Access  Last connection\n"+
		"test-model1                  dummy         local  active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         local  active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         local  destroying  -       never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsUUID(c *tc.C) {
	one := uint64(1)
	s.api.infos[0].Result.Machines = []params.ModelMachineInfo{
		{Id: "0", Hardware: &params.MachineHardware{Cores: &one}}, {Id: "1"},
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        UUID              Cloud/Region  Type   Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 test-model1-UUID  dummy         local  active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         test-model2-UUID  dummy         local  active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  test-model3-UUID  dummy         local  destroying         0      -  -       never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsMachineInfo(c *tc.C) {
	one := uint64(1)
	s.api.infos[0].Result.Machines = []params.ModelMachineInfo{
		{Id: "0", Hardware: &params.MachineHardware{Cores: &one}}, {Id: "1"},
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Type   Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 dummy         local  active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         dummy         local  active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         local  destroying         0      -  -       never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestUnrecognizedArg(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "ERROR unrecognized args: [\"whoops\"]\n")
	s.api.CheckNoCalls(c)
}

func (s *ModelsSuite) TestInvalidUser(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand(), "--user", "+bob")
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`user "+bob" not valid`))
	s.api.CheckNoCalls(c)
}

func (s *ModelsSuite) TestModelsError(c *tc.C) {
	s.api.err = apiservererrors.ErrPerm
	_, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestWithIncompleteModels(c *tc.C) {
	basicAndStatusInfo := createBasicModelInfo()
	basicAndStatusInfo.Status = params.EntityStatus{
		Status: status.Busy,
	}

	basicAndUsersInfo := createBasicModelInfo()
	basicAndUsersInfo.Users = []params.ModelUserInfo{{
		UserName:    "admin",
		DisplayName: "display name",
		Access:      "admin",
	}}

	basicAndMachinesInfo := createBasicModelInfo()
	basicAndMachinesInfo.Machines = []params.ModelMachineInfo{
		{Id: "2"},
		{Id: "12"},
	}

	s.api.infos = []params.ModelInfoResult{
		{Result: createBasicModelInfo()},
		{Result: basicAndStatusInfo},
		{Result: basicAndUsersInfo},
		{Result: basicAndMachinesInfo},
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `
Controller: fake

Model              Cloud/Region           Type   Status  Machines  Access  Last connection
owner/basic-model  altostratus/mid-level  local  -              0  -       never connected
owner/basic-model  altostratus/mid-level  local  busy           0  -       never connected
owner/basic-model  altostratus/mid-level  local  -              0  admin   never connected
owner/basic-model  altostratus/mid-level  local  -              2  -       never connected
`[1:])
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestListModelsWithAgent(c *tc.C) {
	basicInfo := createBasicModelInfo()
	s.assertAgentVersionPresent(c, basicInfo, tc.Contains)
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestListModelsWithNoAgent(c *tc.C) {
	basicInfo := createBasicModelInfo()
	basicInfo.AgentVersion = nil
	s.assertAgentVersionPresent(c, basicInfo, tc.Not(tc.Contains))
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestNoModelsMessage(c *tc.C) {
	assertExpectedOutput := func(context *cmd.Context) {
		c.Assert(cmdtesting.Stdout(context), tc.Equals, `
Controller: fake

Model  Cloud/Region  Type  Status  Access  Last connection
`[1:])
		c.Assert(cmdtesting.Stderr(context), tc.Equals, controller.NoModelsMessage+"\n")
		s.checkAPICalls(c, "ListModelSummaries", "Close")
	}

	s.api.infos = nil
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	assertExpectedOutput(context)

	s.api.ResetCalls()

	s.api.infos = []params.ModelInfoResult{}
	context, err = cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	assertExpectedOutput(context)
}

func (s *ModelsSuite) newCommand() cmd.Command {
	return controller.NewListModelsCommandForTest(s.api, s.api, s.store)
}

func (s *ModelsSuite) assertAgentVersionPresent(c *tc.C, testInfo *params.ModelInfo, checker tc.Checker) {
	s.api.infos = []params.ModelInfoResult{
		{Result: testInfo},
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format=yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), checker, "agent-version")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
}

func (s *ModelsSuite) checkAPICalls(c *tc.C, expectedCalls ...string) {
	s.api.CheckCallNames(c, expectedCalls...)
}

func createBasicModelInfo() *params.ModelInfo {
	agentVersion, _ := semversion.Parse("2.55.5")
	return &params.ModelInfo{
		Name:           "basic-model",
		UUID:           testing.ModelTag.Id(),
		Type:           "iaas",
		ProviderType:   "local",
		ControllerUUID: testing.ControllerTag.Id(),
		IsController:   false,
		Namespace:      "owner",
		Life:           life.Dead,
		CloudTag:       names.NewCloudTag("altostratus").String(),
		CloudRegion:    "mid-level",
		AgentVersion:   &agentVersion,
	}
}

func convert(models []base.UserModel) []params.ModelInfoResult {
	agentVersion, _ := semversion.Parse("2.55.5")
	infoResults := make([]params.ModelInfoResult, len(models))
	for i, model := range models {
		infoResult := params.ModelInfoResult{}
		infoResult.Result = &params.ModelInfo{
			Name:         model.Name,
			UUID:         model.UUID,
			Type:         model.Type.String(),
			Namespace:    model.Namespace,
			CloudTag:     "cloud-dummy",
			ProviderType: "local",
			AgentVersion: &agentVersion,
			Status:       params.EntityStatus{Status: status.Active},
		}
		infoResults[i] = infoResult
	}
	return infoResults
}

func (s *ModelsSuite) TestModelWithUnits(c *tc.C) {
	s.api.units = map[string]int{"test-model2": 3}

	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Type   Status      Units  Access  Last connection\n"+
		"test-model1*                 dummy         local  active      -        read  2015-03-20\n"+
		"carlotta/test-model2         dummy         local  active      3       write  2015-03-01\n"+
		"daiwik@external/test-model3  dummy         local  destroying  -           -  never connected\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsJson(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "json")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `{"models":[{"name":"admin/test-model1","short-name":"test-model1","model-uuid":"test-model1-UUID","model-type":"iaas","controller-uuid":"","controller-name":"fake","is-controller":false,"namespace":"admin","cloud":"dummy","credential":{"name":"one","owner":"bob","cloud":"foo"},"type":"local","life":"","status":{"current":"active"},"access":"read","last-connection":"2015-03-20","agent-version":"2.55.5"},{"name":"carlotta/test-model2","short-name":"test-model2","model-uuid":"test-model2-UUID","model-type":"caas","controller-uuid":"","controller-name":"fake","is-controller":false,"namespace":"carlotta","cloud":"dummy","credential":{"name":"one","owner":"bob","cloud":"foo"},"type":"local","life":"","status":{"current":"active"},"access":"write","last-connection":"2015-03-01","agent-version":"2.55.5"},{"name":"daiwik@external/test-model3","short-name":"test-model3","model-uuid":"test-model3-UUID","model-type":"iaas","controller-uuid":"","controller-name":"fake","is-controller":false,"namespace":"daiwik@external","cloud":"dummy","credential":{"name":"one","owner":"bob","cloud":"foo"},"type":"local","life":"","status":{"current":"destroying"},"access":"","last-connection":"never connected","agent-version":"2.55.5"}],"current-model":"test-model1"}
`)
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsYaml(c *tc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, `
models:
- name: admin/test-model1
  short-name: test-model1
  model-uuid: test-model1-UUID
  model-type: iaas
  controller-uuid: ""
  controller-name: fake
  is-controller: false
  namespace: admin
  cloud: dummy
  credential:
    name: one
    owner: bob
    cloud: foo
  type: local
  life: ""
  status:
    current: active
  access: read
  last-connection: "2015-03-20"
  agent-version: 2.55.5
- name: carlotta/test-model2
  short-name: test-model2
  model-uuid: test-model2-UUID
  model-type: caas
  controller-uuid: ""
  controller-name: fake
  is-controller: false
  namespace: carlotta
  cloud: dummy
  credential:
    name: one
    owner: bob
    cloud: foo
  type: local
  life: ""
  status:
    current: active
  access: write
  last-connection: "2015-03-01"
  agent-version: 2.55.5
- name: daiwik@external/test-model3
  short-name: test-model3
  model-uuid: test-model3-UUID
  model-type: iaas
  controller-uuid: ""
  controller-name: fake
  is-controller: false
  namespace: daiwik@external
  cloud: dummy
  credential:
    name: one
    owner: bob
    cloud: foo
  type: local
  life: ""
  status:
    current: destroying
  access: ""
  last-connection: never connected
  agent-version: 2.55.5
current-model: test-model1
`[1:])
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "")
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestModelsWithOneModelInError(c *tc.C) {
	c.Assert(s.store.Models["fake"].Models, tc.HasLen, 0)
	s.api.infos[2].Error = &params.Error{
		Message: "some model error",
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), tc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                 Cloud/Region  Type   Status  Access  Last connection\n"+
		"test-model1*          dummy         local  active  read    2015-03-20\n"+
		"carlotta/test-model2  dummy         local  active  write   2015-03-01\n")
	c.Assert(cmdtesting.Stderr(context), tc.Equals, "some model error\n")
	c.Assert(s.store.Models["fake"].Models, tc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/test-model1":    {ModelUUID: "test-model1-UUID", ModelType: model.IAAS},
		"carlotta/test-model2": {ModelUUID: "test-model2-UUID", ModelType: model.CAAS},
	})
	s.checkAPICalls(c, "ListModelSummaries", "Close")
}

func (s *ModelsSuite) TestAllModels(c *tc.C) {
	assertAPICallsArgs := func(all bool) {
		s.api.CheckCalls(c, []testhelpers.StubCall{{
			"ListModelSummaries", []interface{}{"admin", all},
		}, {
			"Close", []interface{}{},
		},
		})
	}

	_, err := cmdtesting.RunCommand(c, s.newCommand(), "--all")
	c.Assert(err, tc.ErrorIsNil)
	assertAPICallsArgs(true)

	s.api.ResetCalls()

	_, err = cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, tc.ErrorIsNil)
	assertAPICallsArgs(false)
}
