// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"regexp"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
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

type BaseModelsSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeModelMgrAPIClient
	store *jujuclient.MemStore
}

type ModelsSuiteV3 struct {
	BaseModelsSuite
}

type ModelsSuiteV4 struct {
	BaseModelsSuite
}

var _ = gc.Suite(&ModelsSuiteV3{})
var _ = gc.Suite(&ModelsSuiteV4{})

type fakeModelMgrAPIClient struct {
	*gitjujutesting.Stub

	err   error
	infos []params.ModelInfoResult

	version int
}

func (f *fakeModelMgrAPIClient) BestAPIVersion() int {
	f.MethodCall(f, "BestAPIVersion")
	return f.version
}

func (f *fakeModelMgrAPIClient) Close() error {
	f.MethodCall(f, "Close")
	return nil
}

func (f *fakeModelMgrAPIClient) ListModels(user string) ([]base.UserModel, error) {
	f.MethodCall(f, "ListModels", user)
	if f.err != nil {
		return nil, f.err
	}
	return f.convertInfosToUserModels(), nil
}

func (f *fakeModelMgrAPIClient) AllModels() ([]base.UserModel, error) {
	f.MethodCall(f, "AllModels")
	if f.err != nil {
		return nil, f.err
	}
	return f.convertInfosToUserModels(), nil
}

func (f *fakeModelMgrAPIClient) ListModelSummaries(user names.UserTag) ([]params.ModelSummaryResult, error) {
	f.MethodCall(f, "ListModelSummaries", user)
	if f.err != nil {
		return nil, f.err
	}
	results := make([]params.ModelSummaryResult, len(f.infos))
	for i, info := range f.infos {
		results[i] = params.ModelSummaryResult{}
		if info.Error != nil {
			results[i].Error = info.Error
			continue
		}
		results[i].Result = &params.ModelSummary{
			Name:               info.Result.Name,
			UUID:               info.Result.UUID,
			ControllerUUID:     info.Result.ControllerUUID,
			ProviderType:       info.Result.ProviderType,
			DefaultSeries:      info.Result.DefaultSeries,
			CloudTag:           info.Result.CloudTag,
			CloudRegion:        info.Result.CloudRegion,
			CloudCredentialTag: info.Result.CloudCredentialTag,
			OwnerTag:           info.Result.OwnerTag,
			Life:               info.Result.Life,
			Status:             info.Result.Status,
			Migration:          info.Result.Migration,
			SLA:                info.Result.SLA,
			AgentVersion:       info.Result.AgentVersion,
		}
		if len(info.Result.Users) > 0 {
			results[i].Result.UserAccess = info.Result.Users[0].Access
			results[i].Result.UserLastConnection = info.Result.Users[0].LastConnection
		}
		if len(info.Result.Machines) > 0 {
			results[i].Result.Counts = []params.ModelEntityCount{
				params.ModelEntityCount{params.Machines, int64(len(info.Result.Machines))},
			}
			cores := uint64(0)
			for _, machine := range info.Result.Machines {
				if machine.Hardware != nil && machine.Hardware.Cores != nil {
					cores += *machine.Hardware.Cores
				}
			}
			if cores > 0 {
				results[i].Result.Counts = append(results[i].Result.Counts, params.ModelEntityCount{params.Cores, int64(cores)})
			}
		}
	}
	return results, nil
}

func (f *fakeModelMgrAPIClient) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
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
			models[i] = base.UserModel{UUID: info.Result.UUID}
		}
	}
	return models
}

func (s *BaseModelsSuite) SetUpTest(c *gc.C) {
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
		Stub:    &gitjujutesting.Stub{},
		version: 3,
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
	//2nd model
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

func (s *BaseModelsSuite) TestModelsOwner(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"test-model1*                 dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestModelsNonOwner(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"admin/test-model1*           dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestAllModels(c *gc.C) {
	c.Assert(s.store.Models["fake"].Models, gc.HasLen, 0)
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Access  Last connection\n"+
		"test-model1*                 dummy         active      read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active      write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying  -       never connected\n"+
		"\n")
	c.Assert(s.store.Models["fake"].Models, gc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/test-model1":           jujuclient.ModelDetails{"test-model1-UUID"},
		"carlotta/test-model2":        jujuclient.ModelDetails{"test-model2-UUID"},
		"daiwik@external/test-model3": jujuclient.ModelDetails{"test-model3-UUID"},
	})
	s.checkAPICalls(c, "BestAPIVersion", "AllModels", "Close", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestAllModelsNoneCurrent(c *gc.C) {
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
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestModelsUUID(c *gc.C) {
	one := uint64(1)
	s.api.infos[0].Result.Machines = []params.ModelMachineInfo{
		{Id: "0", Hardware: &params.MachineHardware{Cores: &one}}, {Id: "1"},
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        UUID              Cloud/Region  Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 test-model1-UUID  dummy         active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         test-model2-UUID  dummy         active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  test-model3-UUID  dummy         destroying         0      -  -       never connected\n"+
		"\n")
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestModelsMachineInfo(c *gc.C) {
	one := uint64(1)
	s.api.infos[0].Result.Machines = []params.ModelMachineInfo{
		{Id: "0", Hardware: &params.MachineHardware{Cores: &one}}, {Id: "1"},
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                        Cloud/Region  Status      Machines  Cores  Access  Last connection\n"+
		"test-model1*                 dummy         active             2      1  read    2015-03-20\n"+
		"carlotta/test-model2         dummy         active             0      -  write   2015-03-01\n"+
		"daiwik@external/test-model3  dummy         destroying         0      -  -       never connected\n"+
		"\n")
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

// This test is only needed for older api versions as
// whether the user has an access to a model will be checked on
// the api side and the model data will not be sent.
func (s *BaseModelsSuite) TestAllModelsWithOneUnauthorised(c *gc.C) {
	c.Assert(s.store.Models["fake"].Models, gc.HasLen, 0)
	s.api.infos[2].Error = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}

	context, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, ""+
		"Controller: fake\n"+
		"\n"+
		"Model                 Cloud/Region  Status  Access  Last connection\n"+
		"test-model1*          dummy         active  read    2015-03-20\n"+
		"carlotta/test-model2  dummy         active  write   2015-03-01\n"+
		"\n")
	c.Assert(s.store.Models["fake"].Models, gc.DeepEquals, map[string]jujuclient.ModelDetails{
		"admin/test-model1":    jujuclient.ModelDetails{"test-model1-UUID"},
		"carlotta/test-model2": jujuclient.ModelDetails{"test-model2-UUID"},
	})
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand(), "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
	s.api.CheckNoCalls(c)
}

func (s *BaseModelsSuite) TestInvalidUser(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newCommand(), "--user", "+bob")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`user "+bob" not valid`))
	s.api.CheckNoCalls(c)
}

func (s *BaseModelsSuite) TestModelsError(c *gc.C) {
	s.api.err = common.ErrPerm
	_, err := cmdtesting.RunCommand(c, s.newCommand())
	c.Assert(err, gc.ErrorMatches, ".*: permission denied")
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "Close")
}

func (s *BaseModelsSuite) TestWithIncompleteModels(c *gc.C) {
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

Model              Cloud/Region           Status  Machines  Access  Last connection
owner/basic-model  altostratus/mid-level  -              0  -       never connected
owner/basic-model  altostratus/mid-level  busy           0  -       never connected
owner/basic-model  altostratus/mid-level  -              0  admin   never connected
owner/basic-model  altostratus/mid-level  -              2  -       never connected

`[1:])
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestListModelsWithAgent(c *gc.C) {
	basicInfo := createBasicModelInfo()
	s.assertAgentVersionPresent(c, basicInfo, jc.Contains)
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) TestListModelsWithNoAgent(c *gc.C) {
	basicInfo := createBasicModelInfo()
	basicInfo.AgentVersion = nil
	s.assertAgentVersionPresent(c, basicInfo, gc.Not(jc.Contains))
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *BaseModelsSuite) newCommand() cmd.Command {
	return controller.NewListModelsCommandForTest(s.api, s.api, s.store)
}

func (s *BaseModelsSuite) assertAgentVersionPresent(c *gc.C, testInfo *params.ModelInfo, checker gc.Checker) {
	s.api.infos = []params.ModelInfoResult{
		params.ModelInfoResult{Result: testInfo},
	}
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), checker, "agent-version")
}

func (s *BaseModelsSuite) checkAPICalls(c *gc.C, expectedCalls ...string) {
	actualCalls := []string{}

	switch s.api.version {
	case 4:
		oldCalls := set.NewStrings("ModelInfo", "AllModels", "ListModels")
		// need to add Close here too because in previous implementations it could
		// have been called more than once.
		oldCalls.Add("Close")
		for _, call := range expectedCalls {
			if !oldCalls.Contains(call) {
				actualCalls = append(actualCalls, call)
			}
		}
		actualCalls = append(actualCalls, "ListModelSummaries", "Close")
	default:
		actualCalls = expectedCalls
	}

	s.api.CheckCallNames(c, actualCalls...)
}

func createBasicModelInfo() *params.ModelInfo {
	agentVersion, _ := version.Parse("2.55.5")
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

func convert(models []base.UserModel) []params.ModelInfoResult {
	agentVersion, _ := version.Parse("2.55.5")
	infoResults := make([]params.ModelInfoResult, len(models))
	for i, model := range models {
		infoResult := params.ModelInfoResult{}
		infoResult.Result = &params.ModelInfo{
			Name:         model.Name,
			UUID:         model.UUID,
			OwnerTag:     names.NewUserTag(model.Owner).String(),
			CloudTag:     "cloud-dummy",
			AgentVersion: &agentVersion,
			Status:       params.EntityStatus{Status: status.Active},
		}
		infoResults[i] = infoResult
	}
	return infoResults
}

func (s *ModelsSuiteV3) SetUpTest(c *gc.C) {
	s.BaseModelsSuite.SetUpTest(c)
	// re-run all the test for ModelManager v3
	s.BaseModelsSuite.api.version = 3
}

func (s *ModelsSuiteV3) TestModelsJson(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `{"models":[{"name":"admin/test-model1","short-name":"test-model1","model-uuid":"test-model1-UUID","controller-uuid":"","controller-name":"fake","owner":"admin","cloud":"dummy","life":"","status":{"current":"active"},"users":{"admin":{"access":"read","last-connection":"2015-03-20"}},"agent-version":"2.55.5"},{"name":"carlotta/test-model2","short-name":"test-model2","model-uuid":"test-model2-UUID","controller-uuid":"","controller-name":"fake","owner":"carlotta","cloud":"dummy","life":"","status":{"current":"active"},"users":{"admin":{"access":"write","last-connection":"2015-03-01"}},"agent-version":"2.55.5"},{"name":"daiwik@external/test-model3","short-name":"test-model3","model-uuid":"test-model3-UUID","controller-uuid":"","controller-name":"fake","owner":"daiwik@external","cloud":"dummy","life":"","status":{"current":"destroying"},"agent-version":"2.55.5"}],"current-model":"test-model1"}
`)
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *ModelsSuiteV3) TestModelsYaml(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
models:
- name: admin/test-model1
  short-name: test-model1
  model-uuid: test-model1-UUID
  controller-uuid: ""
  controller-name: fake
  owner: admin
  cloud: dummy
  life: ""
  status:
    current: active
  users:
    admin:
      access: read
      last-connection: 2015-03-20
  agent-version: 2.55.5
- name: carlotta/test-model2
  short-name: test-model2
  model-uuid: test-model2-UUID
  controller-uuid: ""
  controller-name: fake
  owner: carlotta
  cloud: dummy
  life: ""
  status:
    current: active
  users:
    admin:
      access: write
      last-connection: 2015-03-01
  agent-version: 2.55.5
- name: daiwik@external/test-model3
  short-name: test-model3
  model-uuid: test-model3-UUID
  controller-uuid: ""
  controller-name: fake
  owner: daiwik@external
  cloud: dummy
  life: ""
  status:
    current: destroying
  agent-version: 2.55.5
current-model: test-model1
`[1:])
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *ModelsSuiteV4) SetUpTest(c *gc.C) {
	s.BaseModelsSuite.SetUpTest(c)
	// re-run all the test for ModelManager v4
	s.BaseModelsSuite.api.version = 4
}

func (s *ModelsSuiteV4) TestModelsJson(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `{"models":[{"name":"admin/test-model1","short-name":"test-model1","model-uuid":"test-model1-UUID","controller-uuid":"","controller-name":"fake","owner":"admin","cloud":"dummy","life":"","status":{"current":"active"},"access":"read","last-connection":"2015-03-20","agent-version":"2.55.5"},{"name":"carlotta/test-model2","short-name":"test-model2","model-uuid":"test-model2-UUID","controller-uuid":"","controller-name":"fake","owner":"carlotta","cloud":"dummy","life":"","status":{"current":"active"},"access":"write","last-connection":"2015-03-01","agent-version":"2.55.5"},{"name":"daiwik@external/test-model3","short-name":"test-model3","model-uuid":"test-model3-UUID","controller-uuid":"","controller-name":"fake","owner":"daiwik@external","cloud":"dummy","life":"","status":{"current":"destroying"},"access":"","last-connection":"never connected","agent-version":"2.55.5"}],"current-model":"test-model1"}
`)
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}

func (s *ModelsSuiteV4) TestModelsYaml(c *gc.C) {
	context, err := cmdtesting.RunCommand(c, s.newCommand(), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(context), gc.Equals, `
models:
- name: admin/test-model1
  short-name: test-model1
  model-uuid: test-model1-UUID
  controller-uuid: ""
  controller-name: fake
  owner: admin
  cloud: dummy
  life: ""
  status:
    current: active
  access: read
  last-connection: 2015-03-20
  agent-version: 2.55.5
- name: carlotta/test-model2
  short-name: test-model2
  model-uuid: test-model2-UUID
  controller-uuid: ""
  controller-name: fake
  owner: carlotta
  cloud: dummy
  life: ""
  status:
    current: active
  access: write
  last-connection: 2015-03-01
  agent-version: 2.55.5
- name: daiwik@external/test-model3
  short-name: test-model3
  model-uuid: test-model3-UUID
  controller-uuid: ""
  controller-name: fake
  owner: daiwik@external
  cloud: dummy
  life: ""
  status:
    current: destroying
  access: ""
  last-connection: never connected
  agent-version: 2.55.5
current-model: test-model1
`[1:])
	s.checkAPICalls(c, "BestAPIVersion", "ListModels", "ModelInfo", "Close")
}
