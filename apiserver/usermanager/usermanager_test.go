// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/usermanager"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userManagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.UserManagerAPI
	authorizer  apiservertesting.FakeAuthorizer
	adminName   string

	commontesting.BlockHelper
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	adminTag := s.AdminUserTag(c)
	s.adminName = adminTag.Name()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: adminTag,
	}
	var err error
	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *userManagerSuite) TestNewUserManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewMachineTag("1")
	endPoint, err := usermanager.NewUserManagerAPI(s.State, nil, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	result, err := s.usermanager.AddUser(args)
	// Check that the call is succesful
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

func (s *userManagerSuite) TestBlockAddUser(c *gc.C) {
	args := params.AddUsers{
		Users: []params.AddUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	s.BlockAllChanges(c, "TestBlockAddUser")
	result, err := s.usermanager.AddUser(args)
	// Check that the call is blocked
	s.AssertBlocked(c, err, "TestBlockAddUser")
	c.Assert(result.Results, gc.HasLen, 1)
	//check that user is not created
	foobarTag := names.NewLocalUserTag("foobar")
	c.Assert(result.Results[0], gc.DeepEquals, params.AddUserResult{})
	// Check that the call results in a new user being created
	_, err = s.State.User(foobarTag)
	c.Assert(err, gc.ErrorMatches, `user "foobar" not found`)
}

func (s *userManagerSuite) TestAddUserAsNormalUser(c *gc.C) {
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, nil, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, nil, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, nil, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
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

	args := params.UserInfoRequest{
		Entities: []params.Entity{
			{
				Tag: userFoo.Tag().String(),
			}, {
				Tag: userBar.Tag().String(),
			}, {
				Tag: names.NewLocalUserTag("ellie").String(),
			}, {
				Tag: names.NewUserTag("not@remote").String(),
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
			},
		}, {
			user: userBar,
			info: &params.UserInfo{
				Username:    "barfoo",
				DisplayName: "Bar Foo",
				Disabled:    true,
			},
		}, {
			err: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			},
		}, {
			err: &params.Error{
				Message: "permission denied",
				Code:    params.CodeUnauthorized,
			},
		}, {
			err: &params.Error{
				Message: `"not-a-tag" is not a valid tag`,
			},
		},
	} {
		if r.info != nil {
			r.info.DateCreated = r.user.DateCreated()
			r.info.LastConnection = lastLoginPointer(c, r.user)
			r.info.CreatedBy = s.adminName
		}
		expected.Results = append(expected.Results, params.UserInfoResult{Result: r.info, Error: r.err})
	}

	c.Assert(results, jc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoAll(c *gc.C) {
	admin, err := s.State.User(s.AdminUserTag(c))
	c.Assert(err, jc.ErrorIsNil)
	userFoo := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", DisplayName: "Foo Bar"})
	userBar := s.Factory.MakeUser(c, &factory.UserParams{Name: "barfoo", DisplayName: "Bar Foo", Disabled: true})

	args := params.UserInfoRequest{IncludeDisabled: true}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	var expected params.UserInfoResults
	for _, r := range []struct {
		user *state.User
		info *params.UserInfo
	}{
		{
			user: userBar,
			info: &params.UserInfo{
				Username:    "barfoo",
				DisplayName: "Bar Foo",
				Disabled:    true,
			},
		}, {
			user: admin,
			info: &params.UserInfo{
				Username:    s.adminName,
				DisplayName: admin.DisplayName(),
			},
		}, {
			user: userFoo,
			info: &params.UserInfo{
				Username:    "foobar",
				DisplayName: "Foo Bar",
			},
		},
	} {
		r.info.CreatedBy = s.adminName
		r.info.DateCreated = r.user.DateCreated()
		r.info.LastConnection = lastLoginPointer(c, r.user)
		expected.Results = append(expected.Results, params.UserInfoResult{Result: r.info})
	}
	c.Assert(results, jc.DeepEquals, expected)

	results, err = s.usermanager.UserInfo(params.UserInfoRequest{})
	c.Assert(err, jc.ErrorIsNil)
	// Same results as before, but without the deactivated barfoo user
	expected.Results = expected.Results[1:]
	c.Assert(results, jc.DeepEquals, expected)
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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})

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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})

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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, nil, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
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
	alex := s.Factory.MakeUser(c, &factory.UserParams{Name: "alex"})
	barb := s.Factory.MakeUser(c, &factory.UserParams{Name: "barb"})
	usermanager, err := usermanager.NewUserManagerAPI(
		s.State, nil, apiservertesting.FakeAuthorizer{Tag: alex.Tag()})
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
