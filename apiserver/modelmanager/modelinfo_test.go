// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/modelmanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
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
	s.st = &mockState{}
	s.st.model = &mockModel{
		owner: names.NewUserTag("bob@local"),
		cfg:   coretesting.ModelConfig(c),
		users: []*mockModelUser{{
			userName: "admin",
			access:   state.ModelAdminAccess,
		}, {
			userName:    "bob@local",
			displayName: "Bob",
			access:      state.ModelReadAccess,
		}, {
			userName:    "charlotte@local",
			displayName: "Charlotte",
			access:      state.ModelReadAccess,
		}},
	}
	s.modelmanager = modelmanager.NewModelManagerAPIForTest(s.st, &s.authorizer, nil)
}

func (s *modelInfoSuite) TestModelInfo(c *gc.C) {
	s.st.model.users[1].SetErrors(
		nil, state.NeverConnectedError("never connected"),
	)
	info := s.getModelInfo(c)
	c.Assert(info, jc.DeepEquals, params.ModelInfo{
		Name:           "testenv",
		UUID:           s.st.model.cfg.UUID(),
		ControllerUUID: coretesting.ModelTag.Id(),
		OwnerTag:       "user-bob@local",
		ProviderType:   "someprovider",
		DefaultSeries:  coretesting.FakeDefaultSeries,
		Users: []params.ModelUserInfo{{
			UserName:       "admin",
			LastConnection: &time.Time{},
			Access:         params.ModelWriteAccess,
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
		{"GetModel", []interface{}{names.NewModelTag(s.st.model.cfg.UUID())}},
		{"IsControllerAdministrator", []interface{}{names.NewUserTag("admin@local")}},
	})
	s.st.model.CheckCalls(c, []gitjujutesting.StubCall{
		{"Config", nil},
		{"Users", nil},
		{"Owner", nil},
	})
}

func (s *modelInfoSuite) TestModelInfoOwner(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("bob@local")
	info := s.getModelInfo(c)
	c.Assert(info.Users, gc.HasLen, 3)
}

func (s *modelInfoSuite) TestModelInfoNonOwner(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("charlotte@local")
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
	s.authorizer.Tag = names.NewUserTag("nemo@local")
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
	model *mockModel
	owner names.UserTag
	users []*state.ModelUser
}

func (st *mockState) ModelsForUser(user names.UserTag) ([]*state.UserModel, error) {
	st.MethodCall(st, "ModelsForUser", user)
	return nil, st.NextErr()
}

func (st *mockState) IsControllerAdministrator(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdministrator", user)
	return user.Canonical() == "admin@local", st.NextErr()
}

func (st *mockState) NewModel(cfg *config.Config, owner names.UserTag) (*state.Model, *state.State, error) {
	st.MethodCall(st, "NewModel", cfg, owner)
	return nil, nil, st.NextErr()
}

func (st *mockState) ControllerModel() (*state.Model, error) {
	st.MethodCall(st, "ControllerModel")
	return nil, st.NextErr()
}

func (st *mockState) ForModel(tag names.ModelTag) (*state.State, error) {
	st.MethodCall(st, "ForModel", tag)
	return nil, st.NextErr()
}

func (st *mockState) GetModel(tag names.ModelTag) (modelmanager.Model, error) {
	st.MethodCall(st, "GetModel", tag)
	return st.model, st.NextErr()
}

type mockModel struct {
	gitjujutesting.Stub
	owner names.UserTag
	cfg   *config.Config
	users []*mockModelUser
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

type mockModelUser struct {
	gitjujutesting.Stub
	userName       string
	displayName    string
	access         state.ModelAccess
	lastConnection time.Time
}

func (u *mockModelUser) Access() state.ModelAccess {
	u.MethodCall(u, "Access")
	u.PopNoErr()
	return u.access
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
