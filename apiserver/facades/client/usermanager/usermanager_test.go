// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"context"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	coreusertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type userManagerSuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper

	api        *usermanager.UserManagerAPI
	authorizer apiservertesting.FakeAuthorizer
	apiUser    coreuser.User
	resources  *common.Resources

	accessService *MockAccessService
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.setAPIUserAndAuth(c, "admin")
	s.resources = common.NewResources()

	s.BlockHelper = commontesting.NewBlockHelper(s.OpenControllerModelAPI(c))
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	pass := auth.NewPassword("password")
	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        coreusertesting.GenNewName(c, "foobar"),
		DisplayName: "Foo Bar",
		Password:    &pass,
		CreatorUUID: s.apiUser.UUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	}).Return(newUserUUID(c), nil, nil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	sharedModelState := f.MakeModel(c, nil)
	defer func() { _ = sharedModelState.Close() }()

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	result, err := s.api.AddUser(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)

	foobarTag := names.NewLocalUserTag("foobar")
	c.Check(result.Results[0], gc.DeepEquals, params.AddUserResult{Tag: foobarTag.String()})
}

func (s *userManagerSuite) TestAddUserWithSecretKey(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        coreusertesting.GenNewName(c, "foobar"),
		DisplayName: "Foo Bar",
		CreatorUUID: s.apiUser.UUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	}).Return(newUserUUID(c), []byte("secret-key"), nil)

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
		}}}

	result, err := s.api.AddUser(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0], gc.DeepEquals, params.AddUserResult{
		Tag:       names.NewLocalUserTag("foobar").String(),
		SecretKey: []byte("secret-key"),
	})
}

func (s *userManagerSuite) TestBlockAddUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	s.BlockAllChanges(c, "TestBlockAddUser")
	result, err := s.api.AddUser(context.Background(), args)
	s.AssertBlocked(c, err, "TestBlockAddUser")
	c.Assert(result.Results, gc.HasLen, 0)
}

func (s *userManagerSuite) TestAddUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	_ = f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	_, err := s.api.AddUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestDisableUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	exp := s.accessService.EXPECT()
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "barb")).Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "ellie")).Return(errors.NotFound)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "fred@remote")).Return(errors.NotFound)

	args := params.Entities{
		Entities: []params.Entity{
			{"user-alex"},
			{"user-barb"},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}
	result, err := s.api.DisableUser(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{
				Message: "failed to disable user: not found",
			}},
			{Error: &params.Error{
				Message: "failed to disable user: not found",
			}},
			{Error: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			}},
		}})
}

func (s *userManagerSuite) TestBlockDisableUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	args := params.Entities{
		Entities: []params.Entity{
			{"user-alex"},
			{"user-barb"},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}

	s.BlockAllChanges(c, "TestBlockDisableUser")
	_, err := s.api.DisableUser(context.Background(), args)
	s.AssertBlocked(c, err, "TestBlockDisableUser")
}

func (s *userManagerSuite) TestEnableUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	exp := s.accessService.EXPECT()
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "barb")).Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "ellie")).Return(errors.NotFound)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "fred@remote")).Return(errors.NotFound)

	args := params.Entities{
		Entities: []params.Entity{
			{"user-alex"},
			{"user-barb"},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}
	result, err := s.api.EnableUser(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{
				Message: "failed to enable user: not found",
			}},
			{Error: &params.Error{
				Message: "failed to enable user: not found",
			}},
			{Error: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			}},
		}})
}

func (s *userManagerSuite) TestBlockEnableUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := f.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{
			{alex.Tag().String()},
			{barb.Tag().String()},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}

	s.BlockAllChanges(c, "TestBlockEnableUser")
	_, err := s.api.EnableUser(context.Background(), args)
	s.AssertBlocked(c, err, "TestBlockEnableUser")
}

func (s *userManagerSuite) TestDisableUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	_ = f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	barb := f.MakeUser(c, &factory.UserParams{Name: "barb"})

	args := params.Entities{
		Entities: []params.Entity{{barb.Tag().String()}},
	}
	_, err := s.api.DisableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "permission denied")

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsFalse)
}

func (s *userManagerSuite) TestEnableUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	_ = f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	barb := f.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{{barb.Tag().String()}},
	}
	_, err := s.api.EnableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "permission denied")

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsTrue)
}

func (s *userManagerSuite) TestUserInfo(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	exp := s.accessService.EXPECT()
	a := gomock.Any()
	exp.GetUserByName(a, coreusertesting.GenNewName(c, "mary@external")).Return(coreuser.User{
		UUID:     newUserUUID(c),
		Name:     coreusertesting.GenNewName(c, "mary@external"),
		Disabled: false,
	}, nil)
	exp.GetUserByName(a, coreusertesting.GenNewName(c, "foobar")).Return(coreuser.User{
		UUID:     newUserUUID(c),
		Name:     coreusertesting.GenNewName(c, "foobar"),
		Disabled: false,
	}, nil)
	exp.GetUserByName(a, coreusertesting.GenNewName(c, "barfoo")).Return(coreuser.User{
		UUID:     newUserUUID(c),
		Name:     coreusertesting.GenNewName(c, "barfoo"),
		Disabled: true,
	}, nil)
	exp.GetUserByName(a, coreusertesting.GenNewName(c, "ellie")).Return(coreuser.User{}, usererrors.UserNotFound)

	exp.ReadUserAccessLevelForTarget(gomock.Any(), coreusertesting.GenNewName(c, "foobar"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.ControllerUUID,
	}).Return(permission.LoginAccess, nil)

	exp.ReadUserAccessLevelForTarget(gomock.Any(), coreusertesting.GenNewName(c, "mary@external"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.ControllerUUID,
	}).Return(permission.SuperuserAccess, nil)

	args := params.UserInfoRequest{
		Entities: []params.Entity{
			{
				Tag: "user-foobar",
			}, {
				Tag: "user-barfoo",
			}, {
				Tag: "user-ellie",
			}, {
				Tag: "not-a-tag",
			}, {
				Tag: "user-mary@external",
			},
		}}

	results, err := s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	res := results.Results
	c.Assert(res, gc.HasLen, 5)

	c.Assert(res[0].Error, gc.IsNil)
	r0 := res[0].Result
	c.Assert(r0, gc.NotNil)
	c.Check(r0.Username, gc.Equals, "foobar")
	c.Check(r0.Disabled, gc.Equals, false)
	c.Check(r0.Access, gc.Equals, string(permission.LoginAccess))

	c.Assert(res[1].Error, gc.IsNil)
	r1 := res[1].Result
	c.Assert(r1, gc.NotNil)
	c.Check(r1.Username, gc.Equals, "barfoo")
	c.Check(r1.Disabled, gc.Equals, true)
	c.Check(r1.Access, gc.Equals, string(permission.NoAccess))

	c.Check(res[2].Error.Code, gc.Equals, params.CodeUserNotFound)
	c.Check(res[3].Error.Message, gc.Equals, `"not-a-tag" is not a valid tag`)

	c.Assert(res[4].Error, gc.IsNil)
	r4 := res[4].Result
	c.Assert(r4, gc.NotNil)
	c.Check(r4.Username, gc.Equals, "mary@external")
	c.Check(r4.Access, gc.Equals, string(permission.SuperuserAccess))
}

func (s *userManagerSuite) TestUserInfoAll(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	users := []coreuser.User{
		{
			UUID:     newUserUUID(c),
			Name:     coreusertesting.GenNewName(c, "fred"),
			Disabled: false,
		},
	}
	usersIncDisabled := append(users,
		coreuser.User{
			UUID:     newUserUUID(c),
			Name:     coreusertesting.GenNewName(c, "nancy"),
			Disabled: true,
		},
	)

	expected := params.UserInfoResults{
		Results: []params.UserInfoResult{{
			Result: &params.UserInfo{
				Username:       "fred",
				Disabled:       false,
				Access:         "login",
				LastConnection: nil,
			},
		}}}
	expectedIncDisabled := params.UserInfoResults{
		Results: append(expected.Results, params.UserInfoResult{
			Result: &params.UserInfo{
				Username:       "nancy",
				Disabled:       true,
				Access:         "",
				LastConnection: nil,
			},
		}),
	}

	gomock.InOrder(
		s.accessService.EXPECT().GetAllUsers(gomock.Any(), false).Return(users, nil),
		s.accessService.EXPECT().GetAllUsers(gomock.Any(), true).Return(usersIncDisabled, nil),
	)

	// The access service is used only for none-deactivated users, deactivated
	// users have NoPermissions.
	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), coreusertesting.GenNewName(c, "fred"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.ControllerUUID,
	}).Return(permission.LoginAccess, nil).Times(2)

	results, err := s.api.UserInfo(context.Background(), params.UserInfoRequest{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, expected)

	args := params.UserInfoRequest{IncludeDisabled: true}
	results, err = s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, expectedIncDisabled)

}

func (s *userManagerSuite) TestUserInfoNonControllerAdmin(c *gc.C) {
	s.setAPIUserAndAuth(c, "aardvark")
	defer s.setUpAPI(c).Finish()

	fakeCreatorUUID := newUserUUID(c)

	fakeCreator := coreuser.User{
		UUID:        fakeCreatorUUID,
		Name:        coreusertesting.GenNewName(c, "creator"),
		DisplayName: "Creator",
	}

	fakeUUID := newUserUUID(c)

	// CreateAt 5 mins ago
	fakeCreatedAt := time.Now().Add(-5 * time.Minute)

	// LastLogin 2 mins ago
	fakeLastLogin := time.Now().Add(-2 * time.Minute)

	s.accessService.EXPECT().GetUserByName(gomock.Any(), gomock.Any()).Return(coreuser.User{
		UUID:        fakeUUID,
		Name:        coreusertesting.GenNewName(c, "aardvark"),
		DisplayName: "Aard Vark",
		CreatorUUID: fakeCreatorUUID,
		CreatorName: fakeCreator.Name,
		CreatedAt:   fakeCreatedAt,
		LastLogin:   fakeLastLogin,
	}, nil)

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), coreusertesting.GenNewName(c, "aardvark"), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.ControllerUUID,
	}).Return(permission.LoginAccess, nil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f.MakeUser(c, &factory.UserParams{Name: "foobar", DisplayName: "Foo Bar"})
	userAardvark := f.MakeUser(c, &factory.UserParams{Name: "aardvark", DisplayName: "Aard Vark"})

	args := params.UserInfoRequest{Entities: []params.Entity{
		{Tag: userAardvark.Tag().String()},
		{Tag: names.NewUserTag("foobar").String()},
	}}
	results, err := s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	// Non admin users can only see themselves.
	c.Assert(results, jc.DeepEquals, params.UserInfoResults{
		Results: []params.UserInfoResult{
			{
				Result: &params.UserInfo{
					Username:       "aardvark",
					DisplayName:    "Aard Vark",
					Access:         "login",
					CreatedBy:      fakeCreator.Name.Name(),
					DateCreated:    fakeCreatedAt,
					LastConnection: &fakeLastLogin,
				},
			}, {
				Error: &params.Error{
					Message: "permission denied",
					Code:    params.CodeUnauthorized,
				},
			},
		},
	})
}

func (s *userManagerSuite) TestModelUsersInfo(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	testAdmin := jujutesting.AdminUser
	model := s.ControllerModel(c)
	owner, err := s.ControllerModel(c).State().UserAccess(testAdmin, model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	s.accessService.EXPECT().GetModelUsers(gomock.Any(), coreuser.AdminUserName, coremodel.UUID(model.UUID())).Return(
		[]access.ModelUserInfo{{
			Name:           owner.UserName,
			DisplayName:    owner.DisplayName,
			Access:         permission.AdminAccess,
			LastModelLogin: time.Time{},
		}, {
			Name:           coreusertesting.GenNewName(c, "ralphdoe"),
			DisplayName:    "Ralph Doe",
			Access:         permission.AdminAccess,
			LastModelLogin: time.Time{},
		}, {
			Name:           coreusertesting.GenNewName(c, "samsmith"),
			DisplayName:    "Sam Smith",
			Access:         permission.AdminAccess,
			LastModelLogin: time.Time{},
		}, {
			Name:           coreusertesting.GenNewName(c, "bobjohns@ubuntuone"),
			DisplayName:    "Bob Johns",
			Access:         permission.WriteAccess,
			LastModelLogin: time.Time{},
		}, {
			Name:           coreusertesting.GenNewName(c, "nicshaw@idprovider"),
			DisplayName:    "Nic Shaw",
			Access:         permission.WriteAccess,
			LastModelLogin: time.Time{},
		}}, nil,
	)

	results, err := s.api.ModelUserInfo(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: model.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ModelUserInfoResults{
		Results: []params.ModelUserInfoResult{{
			Result: &params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    owner.UserName.Name(),
				DisplayName: owner.DisplayName,
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "ralphdoe",
				DisplayName: "Ralph Doe",
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "samsmith",
				DisplayName: "Sam Smith",
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
				Access:      "write",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "nicshaw@idprovider",
				DisplayName: "Nic Shaw",
				Access:      "write",
			},
		}},
	}

	sort.Sort(ByUserName(expected.Results))
	sort.Sort(ByUserName(results.Results))
	c.Assert(results, jc.DeepEquals, expected)
}

// ByUserName implements sort.Interface for []params.ModelUserInfoResult based on
// the Name field.
type ByUserName []params.ModelUserInfoResult

func (a ByUserName) Len() int           { return len(a) }
func (a ByUserName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByUserName) Less(i, j int) bool { return a[i].Result.UserName < a[j].Result.UserName }

func (s *userManagerSuite) TestSetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().SetPassword(gomock.Any(), coreusertesting.GenNewName(c, "alex"), gomock.Any())

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      "user-alex",
			Password: "new-password",
		}}}
	results, err := s.api.SetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *userManagerSuite) TestBlockSetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.Tag().String(),
			Password: "new-password",
		}}}

	s.BlockAllChanges(c, "TestBlockSetPassword")
	_, err := s.api.SetPassword(context.Background(), args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockSetPassword")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(alex.PasswordValid("new-password"), jc.IsFalse)
}

func (s *userManagerSuite) TestSetPasswordForSelf(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().GetUserByName(gomock.Any(), gomock.Any()).Return(coreuser.User{}, nil).AnyTimes()
	s.accessService.EXPECT().SetPassword(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.Tag().String(),
			Password: "new-password",
		}}}
	results, err := s.api.SetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})

}

func (s *userManagerSuite) TestSetPasswordForOther(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	barb := f.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})

	s.setAPIUserAndAuth(c, alex.Name())
	defer s.setUpAPI(c).Finish()

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      barb.Tag().String(),
			Password: "new-password",
		}}}
	results, err := s.api.SetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		}})

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(barb.PasswordValid("new-password"), jc.IsFalse)
}

func (s *userManagerSuite) TestRemoveUserBadTag(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	tag := "not-a-tag"
	got, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: tag}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Assert(err, gc.Equals, nil)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "\"not-a-tag\" is not a valid tag",
	})
}

func (s *userManagerSuite) TestRemoveUserNonExistent(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	tag := "user-harvey"
	s.accessService.EXPECT().RemoveUser(gomock.Any(), coreusertesting.GenNewName(c, "harvey")).Return(errors.NotFound)

	got, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: tag}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Assert(err, gc.Equals, nil)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "failed to delete user \"harvey\": not found",
		Code:    "not found",
	})
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().RemoveUser(gomock.Any(), coreusertesting.GenNewName(c, "jimmyjam")).Return(nil)

	got, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "user-jimmyjam"}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(got.Results, gc.HasLen, 1)
	c.Check(got.Results[0].Error, gc.IsNil)
}

func (s *userManagerSuite) TestRemoveUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "check")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	// Create a user to delete.
	jjam := f.MakeUser(c, &factory.UserParams{Name: "jimmyjam"})
	// Create a user to delete jjam.
	_ = f.MakeUser(c, &factory.UserParams{
		Name:        "chuck",
		NoModelUser: true,
	})

	_, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")

	// Make sure jjam is still around.
	err = jjam.Refresh()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *userManagerSuite) TestRemoveUserSelfAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "someguy")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	_ = f.MakeUser(c, &factory.UserParams{Name: "someguy"})

	_, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("someguy").String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestRemoveUserAsSelfAdmin(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	expectedError := "cannot delete controller owner \"admin\""

	got, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: jujutesting.AdminUser.String()}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(got.Results, gc.HasLen, 1)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: expectedError,
	})
}

func (s *userManagerSuite) TestRemoveUserBulk(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().RemoveUser(gomock.Any(), coreusertesting.GenNewName(c, "jimmyjam")).Return(nil)
	s.accessService.EXPECT().RemoveUser(gomock.Any(), coreusertesting.GenNewName(c, "alice")).Return(nil)

	got, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "user-jimmyjam"},
			{Tag: "user-alice"},
		}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(got.Results, gc.HasLen, 2)
	var paramErr *params.Error
	c.Check(got.Results[0].Error, jc.DeepEquals, paramErr)
	c.Check(got.Results[1].Error, jc.DeepEquals, paramErr)
}

func (s *userManagerSuite) TestResetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().ResetPassword(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return([]byte("secret-key"), nil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	c.Check(results.Results[0].Tag, gc.Equals, alex.Tag().String())
	c.Check(string(results.Results[0].SecretKey), gc.DeepEquals, "secret-key")
}

func (s *userManagerSuite) TestResetPasswordMultiple(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	gomock.InOrder(
		s.accessService.EXPECT().ResetPassword(gomock.Any(), coreusertesting.GenNewName(c, "alex")),
		s.accessService.EXPECT().ResetPassword(gomock.Any(), coreusertesting.GenNewName(c, "barb")),
	)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	barb := f.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.Tag().String()},
		{Tag: barb.Tag().String()},
	}}
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:       alex.Tag().String(),
			SecretKey: alex.SecretKey(),
		},
		{
			Tag:       barb.Tag().String(),
			SecretKey: barb.SecretKey(),
		},
	})
}

func (s *userManagerSuite) TestBlockResetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)

	s.BlockAllChanges(c, "TestBlockResetPassword")
	_, err := s.api.ResetPassword(context.Background(), args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockResetPassword")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordControllerAdminForSelf(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	alex, err := s.ControllerModel(c).State().User(jujutesting.AdminUser)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	c.Assert(alex.PasswordValid("dummy-secret"), jc.IsTrue)

	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   alex.Tag().String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
	})
	c.Assert(alex.PasswordValid("dummy-secret"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordNotControllerAdmin(c *gc.C) {
	s.setAPIUserAndAuth(c, "dope")
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	alex := f.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	barb := f.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.Tag().String()},
		{Tag: barb.Tag().String()},
	}}
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   alex.Tag().String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
		{
			Tag:   barb.Tag().String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
	})

	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordMixedResult(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.accessService.EXPECT().ResetPassword(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return([]byte("secret-key"), nil)
	s.accessService.EXPECT().ResetPassword(gomock.Any(), coreusertesting.GenNewName(c, "invalid")).Return(nil, errors.NotFound)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "user-invalid"},
		{Tag: "user-alex"},
	}}

	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   "user-invalid",
			Error: apiservererrors.ServerError(errors.NotFound),
		},
		{
			Tag:       "user-alex",
			SecretKey: []byte("secret-key"),
		},
	})
}

func (s *userManagerSuite) TestResetPasswordEmpty(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	results, err := s.api.ResetPassword(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

// setAPIUserAndAuth can be called prior to setUpAPI in order to simulate
// calling the API as the input user. Any name other than "admin" indicates
// that the caller is not an administrator of the controller.
func (s *userManagerSuite) setAPIUserAndAuth(c *gc.C, name string) {
	tag := names.NewUserTag(name)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	s.apiUser = coreuser.User{
		UUID:        newUserUUID(c),
		Name:        coreuser.NameFromTag(tag),
		DisplayName: tag.Name(),
	}
}

func (s *userManagerSuite) setUpAPI(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.accessService = NewMockAccessService(ctrl)

	ctx := facadetest.ModelContext{
		StatePool_: s.StatePool(),
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      s.authorizer,
	}

	var err error
	s.api, err = usermanager.NewAPI(
		ctx.State(),
		s.accessService,
		ctx.Auth(),
		common.NewBlockChecker(ctx.State()),
		ctx.Auth().GetAuthTag().(names.UserTag),
		s.apiUser,
		s.apiUser.Name.Name() == "admin",
		ctx.Logger(),
		s.ControllerUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
