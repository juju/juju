// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"context"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	coreusertesting "github.com/juju/juju/core/user/testing"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/internal/auth"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type userManagerSuite struct {
	jujutesting.ApiServerSuite

	api        *usermanager.UserManagerAPI
	authorizer apiservertesting.FakeAuthorizer
	apiUser    coreuser.User
	resources  *common.Resources

	accessService       *MockAccessService
	modelService        *MockModelService
	blockCommandService *MockBlockCommandService
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.setAPIUserAndAuth(c, "admin")
	s.resources = common.NewResources()
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockAddUser", nil)
	result, err := s.api.AddUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "TestBlockAddUser")
	assertBlocked(c, err, "TestBlockAddUser")
	c.Check(result.Results, gc.HasLen, 0)
}

func (s *userManagerSuite) TestAddUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	exp := s.accessService.EXPECT()
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "barb")).Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "ellie")).Return(errors.NotFound)
	exp.DisableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "fred@remote")).Return(errors.NotFound)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "user-alex"},
			{Tag: "user-barb"},
			{Tag: names.NewLocalUserTag("ellie").String()},
			{Tag: names.NewUserTag("fred@remote").String()},
			{Tag: "not-a-tag"},
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
			{Tag: "user-alex"},
			{Tag: "user-barb"},
			{Tag: names.NewLocalUserTag("ellie").String()},
			{Tag: names.NewUserTag("fred@remote").String()},
			{Tag: "not-a-tag"},
		}}

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockDisableUser", nil)
	_, err := s.api.DisableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "TestBlockDisableUser")
	assertBlocked(c, err, "TestBlockDisableUser")
}

func (s *userManagerSuite) TestEnableUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	exp := s.accessService.EXPECT()
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "alex")).Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "barb")).Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "ellie")).Return(errors.NotFound)
	exp.EnableUserAuthentication(gomock.Any(), coreusertesting.GenNewName(c, "fred@remote")).Return(errors.NotFound)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "user-alex"},
			{Tag: "user-barb"},
			{Tag: names.NewLocalUserTag("ellie").String()},
			{Tag: names.NewUserTag("fred@remote").String()},
			{Tag: "not-a-tag"},
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

	alex := names.NewUserTag("alex")
	barb := names.NewUserTag("barb")

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: alex.String()},
			{Tag: barb.String()},
			{Tag: names.NewLocalUserTag("ellie").String()},
			{Tag: names.NewUserTag("fred@remote").String()},
			{Tag: "not-a-tag"},
		}}

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockEnableUser", nil)
	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.EnableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "TestBlockEnableUser")
	assertBlocked(c, err, "TestBlockEnableUser")
}

func (s *userManagerSuite) TestDisableUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

	barb := names.NewUserTag("barb")

	args := params.Entities{
		Entities: []params.Entity{{Tag: barb.String()}},
	}
	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.DisableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestEnableUserAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "alex")
	defer s.setUpAPI(c).Finish()

	barb := names.NewUserTag("barb")

	args := params.Entities{
		Entities: []params.Entity{{Tag: barb.String()}},
	}
	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.EnableUser(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "permission denied")
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

	userAardvark := names.NewUserTag("aardvark")

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
		Name:        coreuser.NameFromTag(userAardvark),
		DisplayName: "Aard Vark",
		CreatorUUID: fakeCreatorUUID,
		CreatorName: fakeCreator.Name,
		CreatedAt:   fakeCreatedAt,
		LastLogin:   fakeLastLogin,
	}, nil)

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), coreuser.NameFromTag(userAardvark), permission.ID{
		ObjectType: permission.Controller,
		Key:        s.ControllerUUID,
	}).Return(permission.LoginAccess, nil)

	args := params.UserInfoRequest{Entities: []params.Entity{
		{Tag: userAardvark.String()},
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

	owner := names.NewUserTag("owner")

	s.modelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.ApiServerSuite.ControllerModelUUID())).Return(
		[]coremodel.ModelUserInfo{{
			Name:           coreuser.NameFromTag(owner),
			DisplayName:    owner.Name(),
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

	controllerModelTag := names.NewModelTag(s.ApiServerSuite.ControllerModelUUID())

	results, err := s.api.ModelUserInfo(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: controllerModelTag.String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)

	expected := params.ModelUserInfoResults{
		Results: []params.ModelUserInfoResult{{
			Result: &params.ModelUserInfo{
				ModelTag:    controllerModelTag.String(),
				UserName:    owner.Name(),
				DisplayName: owner.Name(),
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    controllerModelTag.String(),
				UserName:    "ralphdoe",
				DisplayName: "Ralph Doe",
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    controllerModelTag.String(),
				UserName:    "samsmith",
				DisplayName: "Sam Smith",
				Access:      "admin",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    controllerModelTag.String(),
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
				Access:      "write",
			},
		}, {
			Result: &params.ModelUserInfo{
				ModelTag:    controllerModelTag.String(),
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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	alex := names.NewUserTag("alex")
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.String(),
			Password: "new-password",
		}}}

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockSetPassword", nil)
	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.SetPassword(context.Background(), args)
	// Check that the call is blocked
	c.Assert(err, gc.ErrorMatches, "TestBlockSetPassword")
	assertBlocked(c, err, "TestBlockSetPassword")
}

func (s *userManagerSuite) TestSetPasswordForSelf(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alex := names.NewUserTag("alex")
	s.accessService.EXPECT().SetPassword(gomock.Any(), coreuser.NameFromTag(alex), auth.NewPassword("new-password")).Return(nil)

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.String(),
			Password: "new-password",
		}}}
	results, err := s.api.SetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
}

func (s *userManagerSuite) TestSetPasswordForOther(c *gc.C) {
	alex := names.NewUserTag("alex")
	barb := names.NewUserTag("barb")
	s.setAPIUserAndAuth(c, alex.Name())
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      barb.String(),
			Password: "new-password",
		}}}
	// Do not expect any calls to the access service as this should fail.
	results, err := s.api.SetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		}})

}

func (s *userManagerSuite) TestRemoveUserBadTag(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

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

func (s *userManagerSuite) expectControllerModelUser(c *gc.C) {
	userUUID := coreusertesting.GenUserUUID(c)
	name, err := coreuser.NewName("admin")
	c.Assert(err, jc.ErrorIsNil)
	s.modelService.EXPECT().ControllerModel(gomock.Any()).Return(coremodel.Model{Owner: userUUID}, nil)
	s.accessService.EXPECT().GetUser(gomock.Any(), userUUID).Return(coreuser.User{Name: name}, nil)
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

	jjam := names.NewUserTag("jimmyjam")

	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: jjam.String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestRemoveUserSelfAsNormalUser(c *gc.C) {
	s.setAPIUserAndAuth(c, "someguy")
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

	// Do not expect any calls to the user service as this should fail.
	_, err := s.api.RemoveUser(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("someguy").String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestRemoveUserAsSelfAdmin(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)
	s.expectControllerModelUser(c)

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

	alex := names.NewUserTag("alex")

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	s.accessService.EXPECT().ResetPassword(gomock.Any(), coreuser.NameFromTag(alex)).Return([]byte("secret-key"), nil)

	args := params.Entities{Entities: []params.Entity{{Tag: alex.String()}}}
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	c.Check(results.Results[0].Tag, gc.Equals, alex.String())
	c.Check(string(results.Results[0].SecretKey), gc.DeepEquals, "secret-key")
}

func (s *userManagerSuite) TestResetPasswordMultiple(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alex := names.NewUserTag("alex")
	barb := names.NewUserTag("barb")
	gomock.InOrder(
		s.accessService.EXPECT().ResetPassword(gomock.Any(), coreuser.NameFromTag(alex)).Return([]byte("alex-secret"), nil),
		s.accessService.EXPECT().ResetPassword(gomock.Any(), coreuser.NameFromTag(barb)).Return([]byte("barb-secret"), nil),
	)

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.String()},
		{Tag: barb.String()},
	}}
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:       alex.String(),
			SecretKey: []byte("alex-secret"),
		},
		{
			Tag:       barb.String(),
			SecretKey: []byte("barb-secret"),
		},
	})
}

func (s *userManagerSuite) TestBlockResetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	alex := names.NewUserTag("alex")
	args := params.Entities{Entities: []params.Entity{{Tag: alex.String()}}}

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("TestBlockResetPassword", nil)
	// Do not expect any calls to the access service as this should fail.
	_, err := s.api.ResetPassword(context.Background(), args)
	// Check that the call is blocked
	c.Assert(err, gc.ErrorMatches, "TestBlockResetPassword")
	assertBlocked(c, err, "TestBlockResetPassword")
}

func (s *userManagerSuite) TestResetPasswordControllerAdminForSelf(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	admin := jujutesting.AdminUser
	args := params.Entities{Entities: []params.Entity{{Tag: admin.String()}}}

	// Do not expect any calls to the access service as this should fail.
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   admin.String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
	})
}

func (s *userManagerSuite) TestResetPasswordNotControllerAdmin(c *gc.C) {
	s.setAPIUserAndAuth(c, "dope")
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alex := names.NewUserTag("alex")
	barb := names.NewUserTag("barb")

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.String()},
		{Tag: barb.String()},
	}}
	// Do not expect any calls to the access service as this should fail.
	results, err := s.api.ResetPassword(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   alex.String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
		{
			Tag:   barb.String(),
			Error: apiservererrors.ServerError(apiservererrors.ErrPerm),
		},
	})
}

func (s *userManagerSuite) TestResetPasswordMixedResult(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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
	s.modelService = NewMockModelService(ctrl)
	s.blockCommandService = NewMockBlockCommandService(ctrl)

	ctx := facadetest.ModelContext{
		Resources_: s.resources,
		Auth_:      s.authorizer,
	}

	var err error
	s.api, err = usermanager.NewAPI(
		s.accessService,
		s.modelService,
		ctx.Auth(),
		common.NewBlockChecker(s.blockCommandService),
		ctx.Auth().GetAuthTag().(names.UserTag),
		s.apiUser,
		s.apiUser.Name.Name() == "admin",
		ctx.Logger(),
		s.ControllerUUID,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}
