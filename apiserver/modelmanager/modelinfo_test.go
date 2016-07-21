// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/metricsender"
	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	coretesting "github.com/juju/juju/testing"
)

type modelInfoSuite struct {
	coretesting.BaseSuite
	authorizer   apiservertesting.FakeAuthorizer
	st           *mockState
	modelmanager *modelmanager.ModelManagerAPI
}

var _ = gc.Suite(&modelInfoSuite{})

func (s *modelInfoSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin@local"),
	}
	s.st = &mockState{
		uuid: coretesting.ModelTag.Id(),
		cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		},
	}
	s.st.model = &mockModel{
		owner: names.NewUserTag("bob@local"),
		cfg:   coretesting.ModelConfig(c),
		life:  state.Dying,
		status: status.StatusInfo{
			Status: status.StatusDestroying,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   description.AdminAccess,
		}, {
			userName:    "bob@local",
			displayName: "Bob",
			access:      description.ReadAccess,
		}, {
			userName:    "charlotte@local",
			displayName: "Charlotte",
			access:      description.ReadAccess,
		}},
	}
	var err error
	s.modelmanager, err = modelmanager.NewModelManagerAPI(s.st, nil, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(s.st, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

func (s *modelInfoSuite) TestModelInfo(c *gc.C) {
	s.st.model.users[1].SetErrors(
		state.NeverConnectedError("never connected"),
		nil, nil, nil, nil,
	)
	info := s.getModelInfo(c)
	c.Assert(info, jc.DeepEquals, params.ModelInfo{
		Name:            "testenv",
		UUID:            s.st.model.cfg.UUID(),
		ControllerUUID:  "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		OwnerTag:        "user-bob@local",
		ProviderType:    "someprovider",
		Cloud:           "some-cloud",
		CloudRegion:     "some-region",
		CloudCredential: "some-credential",
		DefaultSeries:   series.LatestLts(),
		Life:            params.Dying,
		Status: params.EntityStatus{
			Status: status.StatusDestroying,
			Since:  &time.Time{},
		},
		Users: []params.ModelUserInfo{{
			UserName:       "admin",
			LastConnection: &time.Time{},
			Access:         params.ModelAdminAccess,
		}, {
			UserName:       "bob@local",
			DisplayName:    "Bob",
			LastConnection: nil, // never connected
			Access:         params.ModelReadAccess,
		}, {
			UserName:       "charlotte@local",
			DisplayName:    "Charlotte",
			LastConnection: &time.Time{},
			Access:         params.ModelReadAccess,
		}},
	})
	s.st.CheckCalls(c, []gitjujutesting.StubCall{
		{"IsControllerAdministrator", []interface{}{names.NewUserTag("admin@local")}},
		{"ModelUUID", nil},
		{"ForModel", []interface{}{names.NewModelTag(s.st.model.cfg.UUID())}},
		{"Model", nil},
		{"ControllerConfig", nil},
		{"Close", nil},
	})
	s.st.model.CheckCalls(c, []gitjujutesting.StubCall{
		{"Config", nil},
		{"Users", nil},
		{"Status", nil},
		{"Owner", nil},
		{"Life", nil},
		{"Cloud", nil},
		{"CloudRegion", nil},
		{"CloudCredential", nil},
	})
}

func (s *modelInfoSuite) TestModelInfoOwner(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("bob@local"))
	info := s.getModelInfo(c)
	c.Assert(info.Users, gc.HasLen, 3)
}

func (s *modelInfoSuite) TestModelInfoNonOwner(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("charlotte@local"))
	info := s.getModelInfo(c)
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "charlotte@local")
}

func (s *modelInfoSuite) getModelInfo(c *gc.C) params.ModelInfo {
	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{
			names.NewModelTag(s.st.model.cfg.UUID()).String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.NotNil)
	c.Assert(results.Results[0].Error, gc.IsNil)
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
	s.st.model.SetErrors(errors.Errorf("no users for you"))
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

func (s *modelInfoSuite) testModelInfoError(c *gc.C, modelTag, expectedErr string) {
	results, err := s.modelmanager.ModelInfo(params.Entities{
		Entities: []params.Entity{{modelTag}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.IsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, expectedErr)
}

type mockState struct {
	gitjujutesting.Stub

	environs.EnvironConfigGetter
	common.APIHostPortsGetter
	common.ToolsStorageGetter
	common.BlockGetter
	metricsender.MetricsSenderBackend

	uuid            string
	cloud           cloud.Cloud
	model           *mockModel
	controllerModel *mockModel
	users           []*state.ModelUser
	creds           map[string]cloud.Credential
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	UUID string `yaml:"model-uuid"`
}

func (st *mockState) Export() (description.Model, error) {
	return &fakeModelDescription{UUID: st.uuid}, nil
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return st.uuid
}

func (st *mockState) ModelsForUser(user names.UserTag) ([]*state.UserModel, error) {
	st.MethodCall(st, "ModelsForUser", user)
	return nil, st.NextErr()
}

func (st *mockState) IsControllerAdministrator(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdministrator", user)
	if st.controllerModel == nil {
		return user.Canonical() == "admin@local", st.NextErr()
	}
	if st.controllerModel.users == nil {
		return user.Canonical() == "admin@local", st.NextErr()
	}

	for _, u := range st.controllerModel.users {
		if user.Name() == u.UserName() && u.access == description.AdminAccess {
			nextErr := st.NextErr()
			if user.Name() != "admin" {
				panic(user.Name())
			}
			return true, nextErr
		}
	}
	return false, st.NextErr()
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

func (st *mockState) ControllerConfig() (controller.Config, error) {
	st.MethodCall(st, "ControllerConfig")
	return controller.Config{
		controller.ControllerUUIDKey: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	}, st.NextErr()
}

func (st *mockState) ForModel(tag names.ModelTag) (common.ModelManagerBackend, error) {
	st.MethodCall(st, "ForModel", tag)
	return st, st.NextErr()
}

func (st *mockState) Model() (common.Model, error) {
	st.MethodCall(st, "Model")
	return st.model, st.NextErr()
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return st.model.ModelTag()
}

func (st *mockState) AllModels() ([]common.Model, error) {
	st.MethodCall(st, "AllModels")
	return []common.Model{st.model}, st.NextErr()
}

func (st *mockState) Cloud(name string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", name)
	return st.cloud, st.NextErr()
}

func (st *mockState) CloudCredentials(user names.UserTag, cloudName string) (map[string]cloud.Credential, error) {
	st.MethodCall(st, "CloudCredentials", user, cloudName)
	return st.creds, st.NextErr()
}

func (st *mockState) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}

func (st *mockState) AddModelUser(spec state.ModelUserSpec) (*state.ModelUser, error) {
	st.MethodCall(st, "AddModelUser", spec)
	return nil, st.NextErr()
}

func (st *mockState) RemoveModelUser(tag names.UserTag) error {
	st.MethodCall(st, "RemoveModelUser", tag)
	return st.NextErr()
}

func (st *mockState) ModelUser(tag names.UserTag) (*state.ModelUser, error) {
	st.MethodCall(st, "ModelUser", tag)
	return nil, st.NextErr()
}

type mockModel struct {
	gitjujutesting.Stub
	owner  names.UserTag
	life   state.Life
	tag    names.ModelTag
	status status.StatusInfo
	cfg    *config.Config
	users  []*mockModelUser
}

func (m *mockModel) Config() (*config.Config, error) {
	m.MethodCall(m, "Config")
	return m.cfg, m.NextErr()
}

func (m *mockModel) Owner() names.UserTag {
	m.MethodCall(m, "Owner")
	m.PopNoErr()
	return m.owner
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	m.PopNoErr()
	return m.tag
}

func (m *mockModel) Life() state.Life {
	m.MethodCall(m, "Life")
	m.PopNoErr()
	return m.life
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	return m.status, m.NextErr()
}

func (m *mockModel) Cloud() string {
	m.MethodCall(m, "Cloud")
	m.PopNoErr()
	return "some-cloud"
}

func (m *mockModel) CloudRegion() string {
	m.MethodCall(m, "CloudRegion")
	m.PopNoErr()
	return "some-region"
}

func (m *mockModel) CloudCredential() string {
	m.MethodCall(m, "CloudCredential")
	m.PopNoErr()
	return "some-credential"
}

func (m *mockModel) Users() ([]common.ModelUser, error) {
	m.MethodCall(m, "Users")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	users := make([]common.ModelUser, len(m.users))
	for i, user := range m.users {
		users[i] = user
	}
	return users, nil
}

func (m *mockModel) Destroy() error {
	m.MethodCall(m, "Destroy")
	return m.NextErr()
}

func (m *mockModel) DestroyIncludingHosted() error {
	m.MethodCall(m, "DestroyIncludingHosted")
	return m.NextErr()
}

type mockModelUser struct {
	gitjujutesting.Stub
	userName       string
	displayName    string
	lastConnection time.Time
	access         description.Access
}

func (u *mockModelUser) IsAdmin() bool {
	u.MethodCall(u, "IsAdmin")
	u.PopNoErr()
	return u.access == description.AdminAccess
}

func (u *mockModelUser) IsReadOnly() bool {
	u.MethodCall(u, "IsReadOnly")
	u.PopNoErr()
	return u.access == description.ReadAccess
}

func (u *mockModelUser) IsReadWrite() bool {
	u.MethodCall(u, "IsReadWrite")
	u.PopNoErr()
	return u.access == description.WriteAccess
}

func (u *mockModelUser) DisplayName() string {
	u.MethodCall(u, "DisplayName")
	u.PopNoErr()
	return u.displayName
}

func (u *mockModelUser) LastConnection() (time.Time, error) {
	u.MethodCall(u, "LastConnection")
	return u.lastConnection, u.NextErr()
}

func (u *mockModelUser) UserName() string {
	u.MethodCall(u, "UserName")
	u.PopNoErr()
	return u.userName
}

func (u *mockModelUser) UserTag() names.UserTag {
	u.MethodCall(u, "UserTag")
	u.PopNoErr()
	return names.NewUserTag(u.userName)
}
