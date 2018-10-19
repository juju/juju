// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"strings"
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/version"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type modelInfoSuite struct {
	coretesting.BaseSuite
	authorizer   apiservertesting.FakeAuthorizer
	st           *mockState
	ctlrSt       *mockState
	modelmanager *modelmanager.ModelManagerAPI

	callContext context.ProviderCallContext
}

func pUint64(v uint64) *uint64 {
	return &v
}

var _ = gc.Suite(&modelInfoSuite{})

func (s *modelInfoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin@local"),
	}
	s.st = &mockState{
		controllerUUID: coretesting.ControllerTag.Id(),
		cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
		cfgDefaults: config.ModelDefaultAttributes{
			"attr": config.AttributeDefaultValues{
				Default:    "",
				Controller: "val",
				Regions: []config.RegionDefaultValue{{
					Name:  "dummy",
					Value: "val++"}}},
			"attr2": config.AttributeDefaultValues{
				Controller: "val3",
				Default:    "val2",
				Regions: []config.RegionDefaultValue{{
					Name:  "left",
					Value: "spam"}}},
		},
	}
	controllerModel := &mockModel{
		owner: names.NewUserTag("admin@local"),
		life:  state.Alive,
		cfg:   coretesting.ModelConfig(c),
		// This feels kind of wrong as both controller model and
		// default model will end up with the same model tag.
		tag:            coretesting.ModelTag,
		controllerUUID: s.st.controllerUUID,
		status: status.StatusInfo{
			Status: status.Available,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName: "otheruser",
			access:   permission.AdminAccess,
		}},
	}
	s.st.controllerModel = controllerModel

	s.ctlrSt = &mockState{
		model:           controllerModel,
		controllerModel: controllerModel,
	}

	s.st.model = &mockModel{
		owner:          names.NewUserTag("bob@local"),
		cfg:            coretesting.ModelConfig(c),
		tag:            coretesting.ModelTag,
		controllerUUID: s.st.controllerUUID,
		life:           state.Dying,
		status: status.StatusInfo{
			Status: status.Destroying,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName:    "bob",
			displayName: "Bob",
			access:      permission.ReadAccess,
		}, {
			userName:    "charlotte",
			displayName: "Charlotte",
			access:      permission.ReadAccess,
		}, {
			userName:    "mary",
			displayName: "Mary",
			access:      permission.WriteAccess,
		}},
	}
	s.st.machines = []common.Machine{
		&mockMachine{
			id:            "1",
			containerType: "none",
			life:          state.Alive,
			hw:            &instance.HardwareCharacteristics{CpuCores: pUint64(1)},
		},
		&mockMachine{
			id:            "2",
			life:          state.Alive,
			containerType: "lxc",
		},
		&mockMachine{
			id:   "3",
			life: state.Dead,
		},
	}

	s.callContext = context.NewCloudCallContext()

	var err error
	s.modelmanager, err = modelmanager.NewModelManagerAPI(s.st, s.ctlrSt, nil, nil, &s.authorizer, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	var err error
	s.modelmanager, err = modelmanager.NewModelManagerAPI(s.st, s.ctlrSt, nil, nil, s.authorizer, s.st.model, s.callContext)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelInfoSuite) TestModelInfo(c *gc.C) {
	info := s.getModelInfo(c, s.st.model.cfg.UUID())
	expectedAgentVersion, exists := s.st.model.cfg.AgentVersion()
	c.Assert(exists, jc.IsTrue)
	c.Assert(info, jc.DeepEquals, params.ModelInfo{
		Name:               "testmodel",
		UUID:               s.st.model.cfg.UUID(),
		Type:               string(s.st.model.Type()),
		ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		OwnerTag:           "user-bob",
		ProviderType:       "someprovider",
		CloudTag:           "cloud-some-cloud",
		CloudRegion:        "some-region",
		CloudCredentialTag: "cloudcred-some-cloud_bob_some-credential",
		DefaultSeries:      version.SupportedLTS(),
		Life:               params.Dying,
		Status: params.EntityStatus{
			Status: status.Destroying,
			Since:  &time.Time{},
		},
		Users: []params.ModelUserInfo{{
			UserName:       "admin",
			LastConnection: &time.Time{},
			Access:         params.ModelAdminAccess,
		}, {
			UserName:       "bob",
			DisplayName:    "Bob",
			LastConnection: &time.Time{},
			Access:         params.ModelReadAccess,
		}, {
			UserName:       "charlotte",
			DisplayName:    "Charlotte",
			LastConnection: &time.Time{},
			Access:         params.ModelReadAccess,
		}, {
			UserName:       "mary",
			DisplayName:    "Mary",
			LastConnection: &time.Time{},
			Access:         params.ModelWriteAccess,
		}},
		Machines: []params.ModelMachineInfo{{
			Id:       "1",
			Hardware: &params.MachineHardware{Cores: pUint64(1)},
		}, {
			Id: "2",
		}},
		SLA: &params.ModelSLAInfo{
			Level: "essential",
			Owner: "user",
		},
		AgentVersion: &expectedAgentVersion,
	})
	s.st.CheckCalls(c, []gitjujutesting.StubCall{
		{"ControllerTag", nil},
		{"ModelUUID", nil},
		{"GetBackend", []interface{}{s.st.model.cfg.UUID()}},
		{"Model", nil},
		{"AllMachines", nil},
		{"LatestMigration", nil},
	})
	s.st.model.CheckCalls(c, []gitjujutesting.StubCall{
		{"UUID", nil},
		{"Name", nil},
		{"Type", nil},
		{"UUID", nil},
		{"ControllerUUID", nil},
		{"Owner", nil},
		{"Life", nil},
		{"Cloud", nil},
		{"CloudRegion", nil},
		{"CloudCredential", nil},
		{"SLALevel", nil},
		{"SLAOwner", nil},
		{"Life", nil},
		{"Config", nil},
		{"Status", nil},
		{"Users", nil},
		{"ModelTag", nil},
		{"ModelTag", nil},
		{"ModelTag", nil},
		{"ModelTag", nil},
		{"LastModelConnection", []interface{}{names.NewUserTag("admin")}},
		{"LastModelConnection", []interface{}{names.NewLocalUserTag("bob")}},
		{"LastModelConnection", []interface{}{names.NewLocalUserTag("charlotte")}},
		{"LastModelConnection", []interface{}{names.NewLocalUserTag("mary")}},
		{"Type", nil},
	})
}

func (s *modelInfoSuite) TestModelInfoWriteAccess(c *gc.C) {
	mary := names.NewUserTag("mary@local")
	s.authorizer.HasWriteTag = mary
	s.setAPIUser(c, mary)
	info := s.getModelInfo(c, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "mary")
	c.Assert(info.Machines, gc.HasLen, 2)
}

func (s *modelInfoSuite) TestModelInfoNonOwner(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("charlotte@local"))
	info := s.getModelInfo(c, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "charlotte")
	c.Assert(info.Machines, gc.HasLen, 0)
}

func (s *modelInfoSuite) getModelInfo(c *gc.C, modelUUID string) params.ModelInfo {
	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{
			names.NewModelTag(modelUUID).String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Check(results.Results[0].Result, gc.NotNil)
	c.Check(results.Results[0].Error, gc.IsNil)
	return *results.Results[0].Result
}

func (s *modelInfoSuite) TestModelInfoErrorInvalidTag(c *gc.C) {
	s.testModelInfoError(c, "user-bob", `"user-bob" is not a valid model tag`)
}

func (s *modelInfoSuite) TestModelInfoErrorGetModelNotFound(c *gc.C) {
	s.st.SetErrors(errors.NotFoundf("model"))
	s.testModelInfoError(c, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelConfig(c *gc.C) {
	s.st.model.SetErrors(errors.Errorf("no config for you"))
	s.testModelInfoError(c, coretesting.ModelTag.String(), `no config for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelUsers(c *gc.C) {
	s.st.model.SetErrors(
		nil,                               //Config
		nil,                               //Status
		errors.Errorf("no users for you"), // Users
	)
	s.testModelInfoError(c, coretesting.ModelTag.String(), `no users for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoModelUsers(c *gc.C) {
	s.st.model.users = nil
	s.testModelInfoError(c, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoAccess(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("nemo@local"))
	s.testModelInfoError(c, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestRunningMigration(c *gc.C) {
	start := time.Now().Add(-20 * time.Minute)
	s.st.migration = &mockMigration{
		status: "computing optimal bin packing",
		start:  start,
	}

	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})

	c.Assert(err, jc.ErrorIsNil)
	migrationResult := results.Results[0].Result.Migration
	c.Assert(migrationResult.Status, gc.Equals, "computing optimal bin packing")
	c.Assert(*migrationResult.Start, gc.Equals, start)
	c.Assert(migrationResult.End, gc.IsNil)
}

func (s *modelInfoSuite) TestFailedMigration(c *gc.C) {
	start := time.Now().Add(-20 * time.Minute)
	end := time.Now().Add(-10 * time.Minute)
	s.st.migration = &mockMigration{
		status: "couldn't realign alternate time frames",
		start:  start,
		end:    end,
	}

	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})

	c.Assert(err, jc.ErrorIsNil)
	migrationResult := results.Results[0].Result.Migration
	c.Assert(migrationResult.Status, gc.Equals, "couldn't realign alternate time frames")
	c.Assert(*migrationResult.Start, gc.Equals, start)
	c.Assert(*migrationResult.End, gc.Equals, end)
}

func (s *modelInfoSuite) TestNoMigration(c *gc.C) {
	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Result.Migration, gc.IsNil)
}

func (s *modelInfoSuite) TestAliveModelGetsAllInfo(c *gc.C) {
	s.assertSuccess(c, s.st.model.cfg.UUID(), state.Alive, params.Alive)
}

func (s *modelInfoSuite) TestAliveModelWithConfigFailure(c *gc.C) {
	s.st.model.life = state.Alive
	s.setModelConfigError()
	s.testModelInfoError(c, s.st.model.tag.String(), "config not found")
}

func (s *modelInfoSuite) TestAliveModelWithStatusFailure(c *gc.C) {
	s.st.model.life = state.Alive
	s.setModelStatusError()
	s.testModelInfoError(c, s.st.model.tag.String(), "status not found")
}

func (s *modelInfoSuite) TestAliveModelWithUsersFailure(c *gc.C) {
	s.st.model.life = state.Alive
	s.setModelUsersError()
	s.testModelInfoError(c, s.st.model.tag.String(), "users not found")
}

func (s *modelInfoSuite) TestDeadModelGetsAllInfo(c *gc.C) {
	s.assertSuccess(c, s.st.model.cfg.UUID(), state.Dead, params.Dead)
}

func (s *modelInfoSuite) TestDeadModelWithConfigFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Dead,
		expectedLife: params.Dead,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestDeadModelWithStatusFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Dead,
		expectedLife: params.Dead,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestDeadModelWithUsersFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Dead,
		expectedLife: params.Dead,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestDyingModelWithConfigFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Dying,
		expectedLife: params.Dying,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestDyingModelWithStatusFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Dying,
		expectedLife: params.Dying,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestDyingModelWithUsersFailure(c *gc.C) {
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Dying,
		expectedLife: params.Dying,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestImportingModelGetsAllInfo(c *gc.C) {
	s.st.model.migrationStatus = state.MigrationModeImporting
	s.assertSuccess(c, s.st.model.cfg.UUID(), state.Alive, params.Alive)
}

func (s *modelInfoSuite) TestImportingModelWithConfigFailure(c *gc.C) {
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Alive,
		expectedLife: params.Alive,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestImportingModelWithStatusFailure(c *gc.C) {
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Alive,
		expectedLife: params.Alive,
	}
	s.assertSuccessWithMissingData(c, testData)
}

func (s *modelInfoSuite) TestImportingModelWithUsersFailure(c *gc.C) {
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Alive,
		expectedLife: params.Alive,
	}
	s.assertSuccessWithMissingData(c, testData)
}

type incompleteModelInfoTest struct {
	failModel    func()
	desiredLife  state.Life
	expectedLife params.Life
}

func (s *modelInfoSuite) setModelConfigError() {
	s.st.model.SetErrors(errors.NotFoundf("config"))
}

func (s *modelInfoSuite) setModelStatusError() {
	s.st.model.SetErrors(
		nil,                        //Config
		errors.NotFoundf("status"), //Status
	)
}

func (s *modelInfoSuite) setModelUsersError() {
	s.st.model.SetErrors(
		nil,                       //Config
		nil,                       //Status
		errors.NotFoundf("users"), //Users
	)
}

func (s *modelInfoSuite) assertSuccessWithMissingData(c *gc.C, test incompleteModelInfoTest) {
	test.failModel()
	// We do not expect any errors to surface and still want to get basic model info.
	s.assertSuccess(c, s.st.model.cfg.UUID(), test.desiredLife, test.expectedLife)
}

func (s *modelInfoSuite) assertSuccess(c *gc.C, modelUUID string, desiredLife state.Life, expectedLife params.Life) {
	s.st.model.life = desiredLife
	// should get no errors
	info := s.getModelInfo(c, modelUUID)
	c.Assert(info.UUID, gc.Equals, modelUUID)
	c.Assert(info.Life, gc.Equals, expectedLife)
}

func (s *modelInfoSuite) testModelInfoError(c *gc.C, modelTag, expectedErr string) {
	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{modelTag}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.IsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, expectedErr)
}

type unitRetriever interface {
	Unit(name string) (*state.Unit, error)
}

// metricSender defines methods required by the metricsender package.
type metricSender interface {
	MetricsManager() (*state.MetricsManager, error)
	MetricsToSend(batchSize int) ([]*state.MetricBatch, error)
	SetMetricBatchesSent(batchUUIDs []string) error
	CountOfUnsentMetrics() (int, error)
	CountOfSentMetrics() (int, error)
	CleanupOldMetrics() error
}

type mockCaasBroker struct {
	gitjujutesting.Stub
	caas.Broker

	namespaces []string
}

func (m *mockCaasBroker) Namespaces() ([]string, error) {
	m.MethodCall(m, "Namespaces")
	return m.namespaces, nil
}

type mockState struct {
	gitjujutesting.Stub

	environs.EnvironConfigGetter
	common.APIHostPortsForAgentsGetter
	common.ToolsStorageGetter
	common.BlockGetter
	metricSender
	unitRetriever

	controllerUUID  string
	cloud           cloud.Cloud
	clouds          map[names.CloudTag]cloud.Cloud
	cloudUsers      map[string]permission.Access
	model           *mockModel
	controllerModel *mockModel
	users           []permission.UserAccess
	cred            state.Credential
	machines        []common.Machine
	cfgDefaults     config.ModelDefaultAttributes
	blockMsg        string
	block           state.BlockType
	migration       *mockMigration
	modelConfig     *config.Config

	modelDetailsForUser func() ([]state.ModelSummary, error)
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	UUID string `yaml:"model-uuid"`
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return st.model.UUID()
}

func (st *mockState) Name() string {
	st.MethodCall(st, "Name")
	return "test-model"
}

func (st *mockState) ControllerModelUUID() string {
	st.MethodCall(st, "ControllerModelUUID")
	return st.controllerModel.tag.Id()
}

func (st *mockState) ControllerModelTag() names.ModelTag {
	st.MethodCall(st, "ControllerModelTag")
	return st.controllerModel.tag
}

func (st *mockState) Export() (description.Model, error) {
	st.MethodCall(st, "Export")
	return &fakeModelDescription{UUID: st.model.UUID()}, nil
}

func (st *mockState) ExportPartial(cfg state.ExportConfig) (description.Model, error) {
	st.MethodCall(st, "ExportPartial", cfg)
	return st.Export()
}

func (st *mockState) AllModelUUIDs() ([]string, error) {
	st.MethodCall(st, "AllModelUUIDs")
	return []string{st.model.UUID()}, st.NextErr()
}

func (st *mockState) GetBackend(modelUUID string) (common.ModelManagerBackend, func() bool, error) {
	st.MethodCall(st, "GetBackend", modelUUID)
	return st, func() bool { return true }, st.NextErr()
}

func (st *mockState) GetModel(modelUUID string) (common.Model, func() bool, error) {
	st.MethodCall(st, "GetModel", modelUUID)
	return st.model, func() bool { return true }, st.NextErr()
}

func (st *mockState) ModelUUIDsForUser(user names.UserTag) ([]string, error) {
	st.MethodCall(st, "ModelUUIDsForUser", user)
	return nil, st.NextErr()
}

func (st *mockState) AllApplications() ([]common.Application, error) {
	st.MethodCall(st, "AllApplications")
	return nil, st.NextErr()
}

func (st *mockState) AllVolumes() ([]state.Volume, error) {
	st.MethodCall(st, "AllVolumes")
	return nil, st.NextErr()
}

func (st *mockState) AllFilesystems() ([]state.Filesystem, error) {
	st.MethodCall(st, "AllFilesystems")
	return nil, st.NextErr()
}

func (st *mockState) IsControllerAdmin(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdmin", user)
	if st.controllerModel == nil {
		return user.Id() == "admin", st.NextErr()
	}
	if st.controllerModel.users == nil {
		return user.Id() == "admin", st.NextErr()
	}

	for _, u := range st.controllerModel.users {
		if user.Name() == u.userName && u.access == permission.AdminAccess {
			nextErr := st.NextErr()
			if user.Name() != "admin" {
				panic(user.Name())
			}
			return true, nextErr
		}
	}
	return false, st.NextErr()
}

func (st *mockState) GetCloudAccess(cloud string, user names.UserTag) (permission.Access, error) {
	st.MethodCall(st, "GetCloudAccess", user)
	if perm, ok := st.cloudUsers[user.Id()]; ok {
		return perm, nil
	}
	return permission.NoAccess, errors.NotFoundf("user %v", user.Id())
}

func (st *mockState) NewModel(args state.ModelArgs) (common.Model, common.ModelManagerBackend, error) {
	st.MethodCall(st, "NewModel", args)
	st.model.tag = names.NewModelTag(args.Config.UUID())
	return st.model, st, st.NextErr()
}

func (st *mockState) ControllerModel() (common.Model, error) {
	st.MethodCall(st, "ControllerModel")
	return st.controllerModel, st.NextErr()
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag(st.controllerUUID)
}

func (st *mockState) ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.RegionSpec) (map[string]interface{}, error) {
	st.MethodCall(st, "ComposeNewModelConfig")
	attr := make(map[string]interface{})
	for attrName, val := range modelAttr {
		attr[attrName] = val
	}
	attr["something"] = "value"
	return attr, st.NextErr()
}

func (st *mockState) ControllerUUID() string {
	st.MethodCall(st, "ControllerUUID")
	return st.controllerUUID
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	st.MethodCall(st, "ControllerConfig")
	return controller.Config{
		controller.ControllerUUIDKey: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
	}, st.NextErr()
}

func (st *mockState) Model() (common.Model, error) {
	st.MethodCall(st, "Model")
	return st.model, st.NextErr()
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return st.model.ModelTag()
}

func (st *mockState) AllMachines() ([]common.Machine, error) {
	st.MethodCall(st, "AllMachines")
	return st.machines, st.NextErr()
}

func (st *mockState) Clouds() (map[names.CloudTag]cloud.Cloud, error) {
	st.MethodCall(st, "Clouds")
	return st.clouds, st.NextErr()
}

func (st *mockState) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	return st.cloud, st.NextErr()
}

func (st *mockState) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	st.MethodCall(st, "CloudCredential", tag)
	return st.cred, st.NextErr()
}

func (st *mockState) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}

func (st *mockState) AddControllerUser(spec state.UserAccessSpec) (permission.UserAccess, error) {
	st.MethodCall(st, "AddControllerUser", spec)
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) UserAccess(tag names.UserTag, target names.Tag) (permission.UserAccess, error) {
	st.MethodCall(st, "ModelUser", tag, target)
	for _, user := range st.users {
		if user.UserTag != tag {
			continue
		}
		nextErr := st.NextErr()
		if nextErr != nil {
			return permission.UserAccess{}, nextErr
		}
		return user, nil
	}
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) ModelSummariesForUser(user names.UserTag, all bool) ([]state.ModelSummary, error) {
	st.MethodCall(st, "ModelSummariesForUser", user, all)
	return st.modelDetailsForUser()
}

func (st *mockState) ModelBasicInfoForUser(user names.UserTag) ([]state.ModelAccessInfo, error) {
	st.MethodCall(st, "ModelBasicInfoForUser", user)
	return []state.ModelAccessInfo{}, st.NextErr()
}

func (st *mockState) RemoveUserAccess(subject names.UserTag, target names.Tag) error {
	st.MethodCall(st, "RemoveUserAccess", subject, target)
	return st.NextErr()
}

func (st *mockState) SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error) {
	st.MethodCall(st, "SetUserAccess", subject, target, access)
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) ModelConfigDefaultValues() (config.ModelDefaultAttributes, error) {
	st.MethodCall(st, "ModelConfigDefaultValues")
	return st.cfgDefaults, nil
}

func (st *mockState) UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, rspec *environs.RegionSpec) error {
	st.MethodCall(st, "UpdateModelConfigDefaultValues", update, remove, rspec)
	for k, v := range update {
		if rspec != nil {
			adv := st.cfgDefaults[k]
			adv.Regions = append(adv.Regions, config.RegionDefaultValue{
				Name:  rspec.Region,
				Value: v})

		} else {
			st.cfgDefaults[k] = config.AttributeDefaultValues{Controller: v}
		}
	}
	for _, n := range remove {
		if rspec != nil {
			for i, r := range st.cfgDefaults[n].Regions {
				if r.Name == rspec.Region {
					adv := st.cfgDefaults[n]
					adv.Regions = append(adv.Regions[:i], adv.Regions[i+1:]...)
					st.cfgDefaults[n] = adv
				}
			}
		} else {
			if len(st.cfgDefaults[n].Regions) == 0 {
				delete(st.cfgDefaults, n)
			} else {

				st.cfgDefaults[n] = config.AttributeDefaultValues{
					Regions: st.cfgDefaults[n].Regions}
			}
		}
	}
	return nil
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	st.MethodCall(st, "GetBlockForType", t)
	if st.block == t {
		return &mockBlock{t: t, m: st.blockMsg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (st *mockState) ReloadSpaces(environ environs.Environ) error {
	st.MethodCall(st, "ReloadSpaces", environ)
	return st.NextErr()
}

func (st *mockState) DumpAll() (map[string]interface{}, error) {
	st.MethodCall(st, "DumpAll")
	return map[string]interface{}{
		"models": "lots of data",
	}, st.NextErr()
}

func (st *mockState) LatestMigration() (state.ModelMigration, error) {
	st.MethodCall(st, "LatestMigration")
	if st.migration == nil {
		// Handle nil->notfound directly here rather than having to
		// count errors.
		return nil, errors.NotFoundf("")
	}
	return st.migration, st.NextErr()
}

func (st *mockState) SetModelMeterStatus(level, message string) error {
	st.MethodCall(st, "SetModelMeterStatus", level, message)
	return st.NextErr()
}

func (st *mockState) ModelConfig() (*config.Config, error) {
	st.MethodCall(st, "ModelConfig")
	return st.modelConfig, st.NextErr()
}

func (st *mockState) MetricsManager() (*state.MetricsManager, error) {
	st.MethodCall(st, "MetricsManager")
	return nil, errors.New("nope")
}

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (m mockBlock) Id() string { return "" }

func (m mockBlock) Tag() (names.Tag, error) { return names.NewModelTag("mocktesting"), nil }

func (m mockBlock) Type() state.BlockType { return m.t }

func (m mockBlock) Message() string { return m.m }

func (m mockBlock) ModelUUID() string { return "" }

type mockMachine struct {
	common.Machine
	id            string
	life          state.Life
	containerType instance.ContainerType
	hw            *instance.HardwareCharacteristics
}

func (m *mockMachine) Id() string {
	return m.id
}

func (m *mockMachine) Life() state.Life {
	return m.life
}

func (m *mockMachine) ContainerType() instance.ContainerType {
	return m.containerType
}

func (m *mockMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *mockMachine) AgentPresence() (bool, error) {
	return true, nil
}

func (m *mockMachine) InstanceId() (instance.Id, error) {
	return "", nil
}

func (m *mockMachine) WantsVote() bool {
	return false
}

func (m *mockMachine) HasVote() bool {
	return false
}

func (m *mockMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{}, nil
}

type mockModel struct {
	gitjujutesting.Stub
	owner               names.UserTag
	life                state.Life
	tag                 names.ModelTag
	status              status.StatusInfo
	cfg                 *config.Config
	users               []*mockModelUser
	migrationStatus     state.MigrationMode
	controllerUUID      string
	cfgDefaults         config.ModelDefaultAttributes
	setCloudCredentialF func(tag names.CloudCredentialTag) (bool, error)
}

func (m *mockModel) Config() (*config.Config, error) {
	m.MethodCall(m, "Config")
	return m.cfg, m.NextErr()
}

func (m *mockModel) Owner() names.UserTag {
	m.MethodCall(m, "Owner")
	return m.owner
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	return m.tag
}

func (m *mockModel) Type() state.ModelType {
	m.MethodCall(m, "Type")
	return state.ModelTypeIAAS
}

func (m *mockModel) Life() state.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	return m.status, m.NextErr()
}

func (m *mockModel) Cloud() string {
	m.MethodCall(m, "Cloud")
	return "some-cloud"
}

func (m *mockModel) CloudRegion() string {
	m.MethodCall(m, "CloudRegion")
	return "some-region"
}

func (m *mockModel) CloudCredential() (names.CloudCredentialTag, bool) {
	m.MethodCall(m, "CloudCredential")
	return names.NewCloudCredentialTag("some-cloud/bob/some-credential"), true
}

func (m *mockModel) Users() ([]permission.UserAccess, error) {
	m.MethodCall(m, "Users")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	users := make([]permission.UserAccess, len(m.users))
	for i, user := range m.users {
		users[i] = permission.UserAccess{
			UserID:      strings.ToLower(user.userName),
			UserTag:     names.NewUserTag(user.userName),
			Object:      m.ModelTag(),
			Access:      user.access,
			DisplayName: user.displayName,
			UserName:    user.userName,
		}
	}
	return users, nil
}

func (m *mockModel) Destroy(args state.DestroyModelParams) error {
	m.MethodCall(m, "Destroy", args)
	return m.NextErr()
}

func (m *mockModel) SLALevel() string {
	m.MethodCall(m, "SLALevel")
	return "essential"
}

func (m *mockModel) SLAOwner() string {
	m.MethodCall(m, "SLAOwner")
	return "user"
}

func (m *mockModel) ControllerUUID() string {
	m.MethodCall(m, "ControllerUUID")
	return m.controllerUUID
}

func (m *mockModel) UUID() string {
	m.MethodCall(m, "UUID")
	return m.cfg.UUID()
}

func (m *mockModel) Name() string {
	m.MethodCall(m, "Name")
	return m.cfg.Name()
}

func (m *mockModel) MigrationMode() state.MigrationMode {
	m.MethodCall(m, "MigrationMode")
	return m.migrationStatus
}

func (m *mockModel) AddUser(spec state.UserAccessSpec) (permission.UserAccess, error) {
	m.MethodCall(m, "AddUser", spec)
	return permission.UserAccess{}, m.NextErr()
}
func (m *mockModel) LastModelConnection(user names.UserTag) (time.Time, error) {
	m.MethodCall(m, "LastModelConnection", user)
	return time.Time{}, m.NextErr()
}

func (m *mockModel) AutoConfigureContainerNetworking(environ environs.Environ) error {
	m.MethodCall(m, "AutoConfigureContainerNetworking", environ)
	return m.NextErr()
}

func (m *mockModel) ModelConfigDefaultValues() (config.ModelDefaultAttributes, error) {
	m.MethodCall(m, "ModelConfigDefaultValues")
	return m.cfgDefaults, nil
}

func (m *mockModel) getModelDetails() state.ModelSummary {
	cred, _ := m.CloudCredential()
	return state.ModelSummary{
		Name:               m.Name(),
		UUID:               m.UUID(),
		Type:               m.Type(),
		Life:               m.Life(),
		Owner:              m.Owner().Id(),
		ControllerUUID:     m.ControllerUUID(),
		SLALevel:           m.SLALevel(),
		SLAOwner:           m.SLAOwner(),
		CloudTag:           m.Cloud(),
		CloudRegion:        m.CloudRegion(),
		CloudCredentialTag: cred.String(),
	}
}

func (m *mockModel) SetCloudCredential(tag names.CloudCredentialTag) (bool, error) {
	m.MethodCall(m, "SetCloudCredential", tag)
	return m.setCloudCredentialF(tag)
}

type mockModelUser struct {
	gitjujutesting.Stub
	userName       string
	displayName    string
	lastConnection time.Time
	access         permission.Access
}

type mockMigration struct {
	state.ModelMigration

	status string
	start  time.Time
	end    time.Time
}

func (m *mockMigration) StatusMessage() string {
	return m.status
}

func (m *mockMigration) StartTime() time.Time {
	return m.start
}

func (m *mockMigration) EndTime() time.Time {
	return m.end
}
