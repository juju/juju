// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/controller"
	"github.com/juju/juju/apiserver/facades/client/usermanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userManagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.UserManagerAPI
	authorizer  apiservertesting.FakeAuthorizer
	adminName   string
	resources   *common.Resources

	commontesting.BlockHelper
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.resources = common.NewResources()
	adminTag := s.AdminUserTag(c)
	s.adminName = adminTag.Name()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: adminTag,
	}
	var err error
	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *userManagerSuite) TestNewUserManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewMachineTag("1")
	endPoint, err := usermanager.NewUserManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) assertAddUser(c *gc.C, access params.UserAccessPermission, sharedModelTags []string) {
	sharedModelState := s.Factory.MakeModel(c, nil)
	defer sharedModelState.Close()

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	result, err := s.usermanager.AddUser(args)
	// Check that the call is successful
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	foobarTag := names.NewLocalUserTag("foobar")
	c.Assert(result.Results[0], gc.DeepEquals, params.AddUserResult{
		Tag: foobarTag.String()})
	// Check that the call results in a new user being created
	user, err := s.State.User(foobarTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, "foobar")
	c.Assert(user.DisplayName(), gc.Equals, "Foo Bar")
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	s.assertAddUser(c, params.UserAccessPermission(""), nil)
}

func (s *userManagerSuite) TestAddUserWithSecretKey(c *gc.C) {
	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "", // assign secret key
		}}}

	result, err := s.usermanager.AddUser(args)
	// Check that the call is successful
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	foobarTag := names.NewLocalUserTag("foobar")

	// Check that the call results in a new user being created
	user, err := s.State.User(foobarTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, "foobar")
	c.Assert(user.DisplayName(), gc.Equals, "Foo Bar")
	c.Assert(user.SecretKey(), gc.NotNil)
	c.Assert(user.PasswordValid(""), jc.IsFalse)

	// Check that the secret key returned by the API matches what
	// is in state.
	c.Assert(result.Results[0], gc.DeepEquals, params.AddUserResult{
		Tag:       foobarTag.String(),
		SecretKey: user.SecretKey(),
	})
}

func (s *userManagerSuite) TestBlockAddUser(c *gc.C) {
	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	s.BlockAllChanges(c, "TestBlockAddUser")
	result, err := s.usermanager.AddUser(args)
	// Check that the call is blocked.
	s.AssertBlocked(c, err, "TestBlockAddUser")
	// Check that there's no results.
	c.Assert(result.Results, gc.HasLen, 0)
	//check that user is not created.
	foobarTag := names.NewLocalUserTag("foobar")
	// Check that the call results in a new user being created.
	_, err = s.State.User(foobarTag)
	c.Assert(err, gc.ErrorMatches, `user "foobar" not found`)
}

func (s *userManagerSuite) TestAddUserAsNormalUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	_, err = usermanager.AddUser(args)
	c.Assert(err, gc.ErrorMatches, "permission denied")

	_, err = s.State.User(names.NewLocalUserTag("foobar"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *userManagerSuite) TestDisableUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{
			{alex.Tag().String()},
			{barb.Tag().String()},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}
	result, err := s.usermanager.DisableUser(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			}},
			{Error: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			}},
			{Error: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			}},
		}})
	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.IsDisabled(), jc.IsTrue)

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsTrue)
}

func (s *userManagerSuite) TestBlockDisableUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{
			{alex.Tag().String()},
			{barb.Tag().String()},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}

	s.BlockAllChanges(c, "TestBlockDisableUser")
	_, err := s.usermanager.DisableUser(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockDisableUser")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.IsDisabled(), jc.IsFalse)

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsTrue)
}

func (s *userManagerSuite) TestEnableUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{
			{alex.Tag().String()},
			{barb.Tag().String()},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}
	result, err := s.usermanager.EnableUser(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: nil},
			{Error: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			}},
			{Error: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			}},
			{Error: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			}},
		}})
	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.IsDisabled(), jc.IsFalse)

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsFalse)
}

func (s *userManagerSuite) TestBlockEnableUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		Entities: []params.Entity{
			{alex.Tag().String()},
			{barb.Tag().String()},
			{names.NewLocalUserTag("ellie").String()},
			{names.NewUserTag("fred@remote").String()},
			{"not-a-tag"},
		}}

	s.BlockAllChanges(c, "TestBlockEnableUser")
	_, err := s.usermanager.EnableUser(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockEnableUser")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.IsDisabled(), jc.IsFalse)

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsTrue)
}

func (s *userManagerSuite) TestDisableUserAsNormalUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb"})

	args := params.Entities{
		[]params.Entity{{barb.Tag().String()}},
	}
	_, err = usermanager.DisableUser(args)
	c.Assert(err, gc.ErrorMatches, "permission denied")

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsFalse)
}

func (s *userManagerSuite) TestEnableUserAsNormalUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", Disabled: true})

	args := params.Entities{
		[]params.Entity{{barb.Tag().String()}},
	}
	_, err = usermanager.EnableUser(args)
	c.Assert(err, gc.ErrorMatches, "permission denied")

	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(barb.IsDisabled(), jc.IsTrue)
}

func (s *userManagerSuite) TestUserInfo(c *gc.C) {
	userFoo := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", DisplayName: "Foo Bar"})
	userBar := s.Factory.MakeUser(c, &factory.UserParams{Name: "barfoo", DisplayName: "Bar Foo", Disabled: true})
	err := controller.ChangeControllerAccess(
		s.State, s.AdminUserTag(c), names.NewUserTag("fred@external"),
		params.GrantControllerAccess, permission.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = controller.ChangeControllerAccess(
		s.State, s.AdminUserTag(c), names.NewUserTag("everyone@external"),
		params.GrantControllerAccess, permission.SuperuserAccess)
	c.Assert(err, jc.ErrorIsNil)

	args := params.UserInfoRequest{
		Entities: []params.Entity{
			{
				Tag: userFoo.Tag().String(),
			}, {
				Tag: userBar.Tag().String(),
			}, {
				Tag: names.NewLocalUserTag("ellie").String(),
			}, {
				Tag: names.NewUserTag("fred@external").String(),
			}, {
				Tag: names.NewUserTag("mary@external").String(),
			}, {
				Tag: "not-a-tag",
			},
		}}

	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	var expected params.UserInfoResults
	for _, r := range []struct {
		user *state.User
		info *params.UserInfo
		err  *params.Error
	}{
		{
			user: userFoo,
			info: &params.UserInfo{
				Username:    "foobar",
				DisplayName: "Foo Bar",
				Access:      "login",
			},
		}, {
			user: userBar,
			info: &params.UserInfo{
				Username:    "barfoo",
				DisplayName: "Bar Foo",
				Access:      "",
				Disabled:    true,
			},
		}, {
			err: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			},
		}, {
			info: &params.UserInfo{
				Username: "fred@external",
				Access:   "superuser",
			},
		}, {
			info: &params.UserInfo{
				Username: "mary@external",
				Access:   "superuser",
			},
		}, {
			err: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			},
		},
	} {
		if r.info != nil {
			if names.NewUserTag(r.info.Username).IsLocal() {
				r.info.DateCreated = r.user.DateCreated()
				r.info.LastConnection = lastLoginPointer(c, r.user)
				r.info.CreatedBy = s.adminName
			}
		}
		expected.Results = append(expected.Results, params.UserInfoResult{Result: r.info, Error: r.err})
	}

	c.Assert(results, jc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoAll(c *gc.C) {
	admin, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	userFoo := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", DisplayName: "Foo Bar"})
	userAardvark := s.Factory.MakeUser(c, &factory.UserParams{Name: "aardvark", DisplayName: "Aard Vark", Disabled: true})

	args := params.UserInfoRequest{IncludeDisabled: true}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	var expected params.UserInfoResults
	for _, r := range []struct {
		user *state.User
		info *params.UserInfo
	}{{
		user: userAardvark,
		info: &params.UserInfo{
			Username:    "aardvark",
			DisplayName: "Aard Vark",
			Access:      "",
			Disabled:    true,
		},
	}, {
		user: admin,
		info: &params.UserInfo{
			Username:    s.adminName,
			DisplayName: admin.DisplayName(),
			Access:      "superuser",
		},
	}, {
		user: userFoo,
		info: &params.UserInfo{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Access:      "login",
		},
	}} {
		r.info.CreatedBy = s.adminName
		r.info.DateCreated = r.user.DateCreated()
		r.info.LastConnection = lastLoginPointer(c, r.user)
		expected.Results = append(expected.Results, params.UserInfoResult{Result: r.info})
	}
	c.Assert(results, jc.DeepEquals, expected)

	results, err = s.usermanager.UserInfo(params.UserInfoRequest{})
	c.Assert(err, jc.ErrorIsNil)
	// Same results as before, but without the deactivated user
	expected.Results = expected.Results[1:]
	c.Assert(results, jc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoNonControllerAdmin(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", DisplayName: "Foo Bar"})
	userAardvark := s.Factory.MakeUser(c, &factory.UserParams{Name: "aardvark", DisplayName: "Aard Vark"})

	authorizer := apiservertesting.FakeAuthorizer{
		Tag: userAardvark.Tag(),
	}
	usermanager, err := usermanager.NewUserManagerAPI(s.State, s.resources, authorizer)
	c.Assert(err, jc.ErrorIsNil)

	args := params.UserInfoRequest{Entities: []params.Entity{
		{Tag: userAardvark.Tag().String()},
		{Tag: names.NewUserTag("foobar").String()},
	}}
	results, err := usermanager.UserInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	// Non admin users can only see themselves.
	c.Assert(results, jc.DeepEquals, params.UserInfoResults{
		Results: []params.UserInfoResult{
			{
				Result: &params.UserInfo{
					Username:       "aardvark",
					DisplayName:    "Aard Vark",
					Access:         "login",
					CreatedBy:      s.adminName,
					DateCreated:    userAardvark.DateCreated(),
					LastConnection: lastLoginPointer(c, userAardvark),
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
	_, err := s.State.AddControllerUser(state.UserAccessSpec{
		User:      names.NewUserTag("everyone@external"),
		Access:    permission.SuperuserAccess,
		CreatedBy: s.AdminUserTag(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddControllerUser(state.UserAccessSpec{
		User:      names.NewUserTag("aardvark@external"),
		Access:    permission.LoginAccess,
		CreatedBy: s.AdminUserTag(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.UserInfoRequest{Entities: []params.Entity{{Tag: names.NewUserTag("aardvark@external").String()}}}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	// Non admin users can only see themselves.
	c.Assert(results, jc.DeepEquals, params.UserInfoResults{
		Results: []params.UserInfoResult{{Result: &params.UserInfo{
			Username: "aardvark@external",
			Access:   "superuser",
		}}},
	})
}

func lastLoginPointer(c *gc.C, user *state.User) *time.Time {
	lastLogin, err := user.LastLogin()
	if err != nil {
		if state.IsNeverLoggedInError(err) {
			return nil
		}
		c.Fatal(err)
	}
	return &lastLogin
}

func (s *userManagerSuite) TestSetPassword(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.Tag().String(),
			Password: "new-password",
		}}}
	results, err := s.usermanager.SetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(alex.PasswordValid("new-password"), jc.IsTrue)
}

func (s *userManagerSuite) TestBlockSetPassword(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.Tag().String(),
			Password: "new-password",
		}}}

	s.BlockAllChanges(c, "TestBlockSetPassword")
	_, err := s.usermanager.SetPassword(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockSetPassword")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(alex.PasswordValid("new-password"), jc.IsFalse)
}

func (s *userManagerSuite) TestSetPasswordForSelf(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      alex.Tag().String(),
			Password: "new-password",
		}}}
	results, err := usermanager.SetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(alex.PasswordValid("new-password"), jc.IsTrue)
}

func (s *userManagerSuite) TestSetPasswordForOther(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      barb.Tag().String(),
			Password: "new-password",
		}}}
	results, err := usermanager.SetPassword(args)
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
	tag := "not-a-tag"
	got, err := s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: tag}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Assert(err, gc.Equals, nil)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "\"not-a-tag\" is not a valid tag",
	})
}

func (s *userManagerSuite) TestRemoveUserNonExistent(c *gc.C) {
	tag := "user-harvey"
	got, err := s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: tag}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Assert(err, gc.Equals, nil)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: "failed to delete user \"harvey\": user \"harvey\" not found",
		Code:    "not found",
	})
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	// Create a user to delete.
	jjam := s.Factory.MakeUser(c, &factory.UserParams{Name: "jimmyjam"})

	expectedError := fmt.Sprintf("failed to delete user %q: user %q is permanently deleted", jjam.Name(), jjam.Name())

	// Remove the user
	got, err := s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}}})
	c.Assert(got.Results, gc.HasLen, 1)

	c.Check(got.Results[0].Error, gc.IsNil) // Uses gc.IsNil as it's a typed nil.
	c.Assert(err, jc.ErrorIsNil)

	// Check if deleted.
	err = jjam.Refresh()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(jjam.IsDeleted(), jc.IsTrue)

	// Try again and verify we get the expected error.
	got, err = s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}}})
	c.Check(got.Results, gc.HasLen, 1)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: expectedError,
		Code:    "",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *userManagerSuite) TestRemoveUserAsNormalUser(c *gc.C) {
	// Create a user to delete.
	jjam := s.Factory.MakeUser(c, &factory.UserParams{Name: "jimmyjam"})
	// Create a user to delete jjam.
	chuck := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "chuck",
		NoModelUser: true,
	})

	// Authenticate as chuck.
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{
			Tag: chuck.Tag(),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the user exists.
	ui, err := s.usermanager.UserInfo(params.UserInfoRequest{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(ui.Results, gc.HasLen, 1)
	c.Assert(ui.Results[0].Result.Username, gc.DeepEquals, jjam.Name())

	// Remove jjam as chuck and fail.
	_, err = usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")

	// Make sure jjam is still around.
	err = jjam.Refresh()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *userManagerSuite) TestRemoveUserSelfAsNormalUser(c *gc.C) {
	// Create a user to delete.
	jjam := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "jimmyjam",
		NoModelUser: true,
	})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{
			Tag: jjam.Tag(),
		})
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the user exists.
	ui, err := s.usermanager.UserInfo(params.UserInfoRequest{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ui.Results, gc.HasLen, 1)
	c.Assert(ui.Results[0].Result.Username, gc.DeepEquals, jjam.Name())

	// Remove the user as the user
	_, err = usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: jjam.Tag().String()}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")

	// Check if deleted.
	err = jjam.Refresh()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *userManagerSuite) TestRemoveUserAsSelfAdmin(c *gc.C) {

	expectedError := "cannot delete controller owner \"admin\""

	// Remove admin as admin.
	got, err := s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: s.AdminUserTag(c).String()}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: expectedError,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Try again to see if we succeeded.
	got, err = s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{{Tag: s.AdminUserTag(c).String()}}})
	c.Assert(got.Results, gc.HasLen, 1)
	c.Check(got.Results[0].Error, jc.DeepEquals, &params.Error{
		Message: expectedError,
	})
	c.Assert(err, jc.ErrorIsNil)

	ui, err := s.usermanager.UserInfo(params.UserInfoRequest{})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(ui.Results, gc.HasLen, 1)

}

func (s *userManagerSuite) TestRemoveUserBulkSharedModels(c *gc.C) {
	// Create users.
	jjam := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "jimmyjam",
	})
	alice := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "alice",
	})
	bob := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "bob",
	})

	// Get a handle on the current model.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	users, err := model.Users()

	// Make sure the users exist.
	var userNames []string
	for _, u := range users {
		userNames = append(userNames, u.UserTag.Name())
	}
	c.Assert(userNames, jc.SameContents, []string{"admin", jjam.Name(), alice.Name(), bob.Name()})

	// Remove 2 users.
	got, err := s.usermanager.RemoveUser(params.Entities{
		Entities: []params.Entity{
			{Tag: jjam.Tag().String()},
			{Tag: alice.Tag().String()},
		}})
	c.Check(got.Results, gc.HasLen, 2)
	var paramErr *params.Error
	c.Check(got.Results[0].Error, jc.DeepEquals, paramErr)
	c.Check(got.Results[1].Error, jc.DeepEquals, paramErr)
	c.Assert(err, jc.ErrorIsNil)

	// Make sure users were deleted.
	err = jjam.Refresh()
	c.Assert(jjam.IsDeleted(), jc.IsTrue)
	err = alice.Refresh()
	c.Assert(alice.IsDeleted(), jc.IsTrue)

}

func (s *userManagerSuite) TestResetPassword(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	results, err := s.usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Tag, gc.DeepEquals, alex.Tag().String())
	c.Assert(results.Results[0].SecretKey, gc.DeepEquals, alex.SecretKey())
	c.Assert(alex.PasswordValid("password"), jc.IsFalse)
}

func (s *userManagerSuite) TestResetPasswordMultiple(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.Tag().String()},
		{Tag: barb.Tag().String()},
	}}
	results, err := s.usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = barb.Refresh()
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
	c.Assert(alex.PasswordValid("password"), jc.IsFalse)
	c.Assert(barb.PasswordValid("password"), jc.IsFalse)
}

func (s *userManagerSuite) TestBlockResetPassword(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)

	s.BlockAllChanges(c, "TestBlockResetPassword")
	_, err := s.usermanager.ResetPassword(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockResetPassword")

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordControllerAdminForSelf(c *gc.C) {
	alex, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{{Tag: alex.Tag().String()}}}
	c.Assert(alex.PasswordValid("dummy-secret"), jc.IsTrue)

	results, err := s.usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   alex.Tag().String(),
			Error: common.ServerError(common.ErrPerm),
		},
	})
	c.Assert(alex.PasswordValid("dummy-secret"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordNotControllerAdmin(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb", NoModelUser: true})
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, s.resources, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: alex.Tag().String()},
		{Tag: barb.Tag().String()},
	}}
	results, err := usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)

	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = barb.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   alex.Tag().String(),
			Error: common.ServerError(common.ErrPerm),
		},
		{
			Tag:   barb.Tag().String(),
			Error: common.ServerError(common.ErrPerm),
		},
	})

	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	c.Assert(barb.PasswordValid("password"), jc.IsTrue)
}

func (s *userManagerSuite) TestResetPasswordFail(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true, Disabled: true})
	args := params.Entities{Entities: []params.Entity{
		{Tag: "user-invalid"},
		{Tag: alex.Tag().String()},
	}}

	results, err := s.usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   "user-invalid",
			Error: common.ServerError(common.ErrPerm),
		},
		{
			Tag:   alex.Tag().String(),
			Error: common.ServerError(fmt.Errorf("cannot reset password for user \"alex\": user deactivated")),
		},
	})
}

func (s *userManagerSuite) TestResetPasswordMixedResult(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex", NoModelUser: true})
	c.Assert(alex.PasswordValid("password"), jc.IsTrue)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "user-invalid"},
		{Tag: alex.Tag().String()},
	}}

	results, err := s.usermanager.ResetPassword(args)
	c.Assert(err, jc.ErrorIsNil)
	err = alex.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.AddUserResult{
		{
			Tag:   "user-invalid",
			Error: common.ServerError(common.ErrPerm),
		},
		{
			Tag:       alex.Tag().String(),
			SecretKey: alex.SecretKey(),
		},
	})
	c.Assert(alex.PasswordValid("password"), jc.IsFalse)
}

func (s *userManagerSuite) TestResetPasswordEmpty(c *gc.C) {
	results, err := s.usermanager.ResetPassword(params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}
