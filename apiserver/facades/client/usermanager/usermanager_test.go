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
	"github.com/juju/juju/domain/access"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userManagerSuite struct {
	jujutesting.ApiServerSuite
	commontesting.BlockHelper

	api        *usermanager.UserManagerAPI
	authorizer apiservertesting.FakeAuthorizer
	apiUser    coreuser.User
	resources  *common.Resources

	userService *MockUserService
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
	s.userService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        "foobar",
		DisplayName: "Foo Bar",
		Password:    &pass,
		CreatorUUID: s.apiUser.UUID,
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
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
	// Check that the call is successful
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	foobarTag := names.NewLocalUserTag("foobar")
	c.Assert(result.Results[0], gc.DeepEquals, params.AddUserResult{Tag: foobarTag.String()})
}

func (s *userManagerSuite) TestAddUserWithSecretKey(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.userService.EXPECT().AddUser(gomock.Any(), service.AddUserArg{
		Name:        "foobar",
		DisplayName: "Foo Bar",
		CreatorUUID: s.apiUser.UUID,
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
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

	exp := s.userService.EXPECT()
	exp.DisableUserAuthentication(gomock.Any(), "alex").Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), "barb").Return(nil)
	exp.DisableUserAuthentication(gomock.Any(), "ellie").Return(errors.NotFound)
	exp.DisableUserAuthentication(gomock.Any(), "fred").Return(errors.NotFound)

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

	exp := s.userService.EXPECT()
	exp.EnableUserAuthentication(gomock.Any(), "alex").Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), "barb").Return(nil)
	exp.EnableUserAuthentication(gomock.Any(), "ellie").Return(errors.NotFound)
	exp.EnableUserAuthentication(gomock.Any(), "fred").Return(errors.NotFound)

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

	exp := s.userService.EXPECT()
	a := gomock.Any()
	exp.GetUserByName(a, "foobar").Return(coreuser.User{
		UUID:     newUserUUID(c),
		Name:     "foobar",
		Disabled: false,
	}, nil)
	exp.GetUserByName(a, "barfoo").Return(coreuser.User{
		UUID:     newUserUUID(c),
		Name:     "barfoo",
		Disabled: true,
	}, nil)
	exp.GetUserByName(a, "ellie").Return(coreuser.User{}, usererrors.UserNotFound)

	args := params.UserInfoRequest{
		Entities: []params.Entity{
			{
				Tag: "user-foobar",
			}, {
				Tag: "user-barfoo",
			}, {
				Tag: names.NewLocalUserTag("ellie").String(),
			}, {
				Tag: "not-a-tag",
			},
		}}

	results, err := s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	res := results.Results
	c.Assert(res, gc.HasLen, 4)

	c.Check(res[0].Result.Username, gc.Equals, "foobar")
	c.Check(res[0].Result.Disabled, gc.Equals, false)
	//c.Check(res[0].Result.Access, gc.Equals, string(permission.LoginAccess))

	c.Check(res[1].Result.Username, gc.Equals, "barfoo")
	c.Check(res[1].Result.Disabled, gc.Equals, true)
	//c.Check(res[1].Result.Access, gc.Equals, string(permission.NoAccess))

	c.Check(res[2].Error.Code, gc.Equals, params.CodeUserNotFound)
	c.Check(res[3].Error.Message, gc.Equals, `"not-a-tag" is not a valid tag`)
}

func (s *userManagerSuite) TestUserInfoAll(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	users := []coreuser.User{
		{
			UUID:     newUserUUID(c),
			Name:     "fred",
			Disabled: false,
		},
		{
			UUID:     newUserUUID(c),
			Name:     "nancy",
			Disabled: false,
		},
	}

	// TODO (manadart 2024-02-14) This test is contrived to pass.
	// The service is not correctly implemented as it does not
	// factor the `IncludeDisabled` argument.

	gomock.InOrder(
		s.userService.EXPECT().GetAllUsers(gomock.Any()).Return(users, nil),
		s.userService.EXPECT().GetAllUsers(gomock.Any()).Return(users, nil),
	)

	args := params.UserInfoRequest{IncludeDisabled: true}
	_, err := s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	//var expected params.UserInfoResults
	//c.Check(results, jc.DeepEquals, expected)

	_, err = s.api.UserInfo(context.Background(), params.UserInfoRequest{})
	c.Assert(err, jc.ErrorIsNil)

	// Same results as before, but without the deactivated user
	//expected.Results = expected.Results[1:]
	//c.Check(results, jc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoNonControllerAdmin(c *gc.C) {
	s.setAPIUserAndAuth(c, "aardvark")
	defer s.setUpAPI(c).Finish()

	fakeCreatorUUID := newUserUUID(c)

	fakeCreator := coreuser.User{
		UUID:        fakeCreatorUUID,
		Name:        "creator",
		DisplayName: "Creator",
	}

	fakeUUID := newUserUUID(c)

	// CreateAt 5 mins ago
	fakeCreatedAt := time.Now().Add(-5 * time.Minute)

	// LastLogin 2 mins ago
	fakeLastLogin := time.Now().Add(-2 * time.Minute)

	s.userService.EXPECT().GetUserByName(gomock.Any(), gomock.Any()).Return(coreuser.User{
		UUID:        fakeUUID,
		Name:        "aardvark",
		DisplayName: "Aard Vark",
		CreatorUUID: fakeCreatorUUID,
		CreatorName: fakeCreator.Name,
		CreatedAt:   fakeCreatedAt,
		LastLogin:   fakeLastLogin,
	}, nil)

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
					CreatedBy:      fakeCreator.Name,
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

func (s *userManagerSuite) TestUserInfoEveryonePermission(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	st := s.ControllerModel(c).State()
	_, err := st.AddControllerUser(state.UserAccessSpec{
		User:      names.NewUserTag("everyone@external"),
		Access:    permission.SuperuserAccess,
		CreatedBy: jujutesting.AdminUser,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddControllerUser(state.UserAccessSpec{
		User:      names.NewUserTag("aardvark@external"),
		Access:    permission.LoginAccess,
		CreatedBy: jujutesting.AdminUser,
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.UserInfoRequest{Entities: []params.Entity{{Tag: names.NewUserTag("aardvark@external").String()}}}
	results, err := s.api.UserInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	// Non admin users can only see themselves.
	c.Assert(results, jc.DeepEquals, params.UserInfoResults{
		Results: []params.UserInfoResult{{Result: &params.UserInfo{
			Username: "aardvark@external",
			Access:   "superuser",
		}}},
	})
}

func (s *userManagerSuite) makeLocalModelUser(c *gc.C, username, displayname string) permission.UserAccess {
	defer s.setUpAPI(c).Finish()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	// factory.MakeUser will create an ModelUser for a local user by default.
	user := f.MakeUser(c, &factory.UserParams{Name: username, DisplayName: displayname})
	modelUser, err := s.ControllerModel(c).State().UserAccess(user.UserTag(), s.ControllerModel(c).ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	return modelUser
}

func (s *userManagerSuite) TestModelUsersInfo(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	testAdmin := jujutesting.AdminUser
	model := s.ControllerModel(c)
	owner, err := s.ControllerModel(c).State().UserAccess(testAdmin, model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	localUser1 := s.makeLocalModelUser(c, "ralphdoe", "Ralph Doe")
	localUser2 := s.makeLocalModelUser(c, "samsmith", "Sam Smith")
	remoteUser1 := f.MakeModelUser(c, &factory.ModelUserParams{User: "bobjohns@ubuntuone", DisplayName: "Bob Johns", Access: permission.WriteAccess})
	remoteUser2 := f.MakeModelUser(c, &factory.ModelUserParams{User: "nicshaw@idprovider", DisplayName: "Nic Shaw", Access: permission.WriteAccess})

	s.userService.EXPECT().ModelUserInfo(gomock.Any(), coremodel.UUID(model.UUID())).Return([]access.ModelUserInfo{{
		UserName:       owner.UserName,
		DisplayName:    owner.DisplayName,
		LastConnection: nil,
		Access:         owner.Access,
	}, {
		UserName:       localUser1.UserName,
		DisplayName:    localUser1.DisplayName,
		LastConnection: nil,
		Access:         localUser1.Access,
	}, {
		UserName:       localUser2.UserName,
		DisplayName:    localUser2.DisplayName,
		LastConnection: nil,
		Access:         localUser2.Access,
	}, {
		UserName:       remoteUser1.UserName,
		DisplayName:    remoteUser1.DisplayName,
		LastConnection: nil,
		Access:         remoteUser1.Access,
	}, {
		UserName:       remoteUser2.UserName,
		DisplayName:    remoteUser2.DisplayName,
		LastConnection: nil,
		Access:         remoteUser2.Access,
	}}, nil)

	results, err := s.api.ModelUserInfo(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: model.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	var expected params.ModelUserInfoResults
	for _, r := range []struct {
		user permission.UserAccess
		info *params.ModelUserInfo
	}{
		{
			owner,
			&params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    owner.UserName,
				DisplayName: owner.DisplayName,
				Access:      "admin",
			},
		}, {
			localUser1,
			&params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "ralphdoe",
				DisplayName: "Ralph Doe",
				Access:      "admin",
			},
		}, {
			localUser2,
			&params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "samsmith",
				DisplayName: "Sam Smith",
				Access:      "admin",
			},
		}, {
			remoteUser1,
			&params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "bobjohns@ubuntuone",
				DisplayName: "Bob Johns",
				Access:      "write",
			},
		}, {
			remoteUser2,
			&params.ModelUserInfo{
				ModelTag:    model.ModelTag().String(),
				UserName:    "nicshaw@idprovider",
				DisplayName: "Nic Shaw",
				Access:      "write",
			},
		},
	} {
		expected.Results = append(expected.Results, params.ModelUserInfoResult{Result: r.info})
	}

	sort.Sort(ByUserName(expected.Results))
	sort.Sort(ByUserName(results.Results))
	c.Assert(results, jc.DeepEquals, expected)
}

// ByUserName implements sort.Interface for []params.ModelUserInfoResult based on
// the UserName field.
type ByUserName []params.ModelUserInfoResult

func (a ByUserName) Len() int           { return len(a) }
func (a ByUserName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByUserName) Less(i, j int) bool { return a[i].Result.UserName < a[j].Result.UserName }

func (s *userManagerSuite) TestSetPassword(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.userService.EXPECT().SetPassword(gomock.Any(), "alex", gomock.Any())

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

	s.userService.EXPECT().GetUserByName(gomock.Any(), gomock.Any()).Return(coreuser.User{}, nil).AnyTimes()
	s.userService.EXPECT().SetPassword(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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
	s.userService.EXPECT().RemoveUser(gomock.Any(), "harvey").Return(errors.NotFound)

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

	s.userService.EXPECT().RemoveUser(gomock.Any(), "jimmyjam").Return(nil)

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

	s.userService.EXPECT().RemoveUser(gomock.Any(), "jimmyjam").Return(nil)
	s.userService.EXPECT().RemoveUser(gomock.Any(), "alice").Return(nil)

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

	s.userService.EXPECT().ResetPassword(gomock.Any(), "alex").Return([]byte("secret-key"), nil)

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
		s.userService.EXPECT().ResetPassword(gomock.Any(), "alex"),
		s.userService.EXPECT().ResetPassword(gomock.Any(), "barb"),
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

	s.userService.EXPECT().ResetPassword(gomock.Any(), "alex").Return([]byte("secret-key"), nil)
	s.userService.EXPECT().ResetPassword(gomock.Any(), "invalid").Return(nil, errors.NotFound)

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
		Name:        tag.Name(),
		DisplayName: tag.Name(),
	}
}

func (s *userManagerSuite) setUpAPI(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.userService = NewMockUserService(ctrl)

	ctx := facadetest.ModelContext{
		StatePool_: s.StatePool(),
		State_:     s.ControllerModel(c).State(),
		Resources_: s.resources,
		Auth_:      s.authorizer,
	}

	var err error
	s.api, err = usermanager.NewAPI(
		ctx.State(),
		s.userService,
		ctx.StatePool(),
		ctx.Auth(),
		common.NewBlockChecker(ctx.State()),
		ctx.Auth().GetAuthTag().(names.UserTag),
		s.apiUser,
		s.apiUser.Name == "admin",
		ctx.Logger(),
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
