// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/usermanager"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type usermanagerSuite struct{}

func TestUsermanagerSuite(t *stdtesting.T) { tc.Run(t, &usermanagerSuite{}) }
func (s *usermanagerSuite) TestAddExistingUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.AddUsers{
		Users: []params.AddUser{{Username: "foobar", DisplayName: "Foo Bar", Password: "password"}},
	}

	result := new(params.AddUserResults)
	results := params.AddUserResults{
		Results: []params.AddUserResult{
			{
				Tag:       "user-foobar",
				SecretKey: []byte("passwedfdd"),
				Error:     apiservererrors.ServerError(errors.Annotate(errors.New("user foobar already exists"), "failed to create user")),
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddUser", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, _, err := client.AddUser(c.Context(), "foobar", "Foo Bar", "password")
	c.Assert(err, tc.ErrorMatches, "failed to create user: user foobar already exists")
}

func (s *usermanagerSuite) TestAddUserResponseError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.AddUsers{
		Users: []params.AddUser{{Username: "foobar", DisplayName: "Foo Bar", Password: "password"}},
	}

	result := new(params.AddUserResults)
	results := params.AddUserResults{
		Results: make([]params.AddUserResult, len(args.Users)),
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddUser", args, result).SetArg(3, results).Return(errors.New("call error"))

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, _, err := client.AddUser(c.Context(), "foobar", "Foo Bar", "password")
	c.Assert(err, tc.ErrorMatches, "call error")
}

func (s *usermanagerSuite) TestAddUserResultCount(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.AddUsers{
		Users: []params.AddUser{{Username: "foobar", DisplayName: "Foo Bar", Password: "password"}},
	}

	result := new(params.AddUserResults)
	results := params.AddUserResults{
		Results: make([]params.AddUserResult, 2),
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddUser", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, _, err := client.AddUser(c.Context(), "foobar", "Foo Bar", "password")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *usermanagerSuite) TestRemoveUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	arg := params.Entities{
		Entities: []params.Entity{{Tag: "user-jjam"}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveUser", arg, result).SetArg(3, results).Return(nil)
	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	// Delete the user.
	err := client.RemoveUser(c.Context(), "jjam")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *usermanagerSuite) TestDisableUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	user := names.NewUserTag("foobar")
	args := params.Entities{
		Entities: []params.Entity{{Tag: "user-foobar"}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DisableUser", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.DisableUser(c.Context(), user.Name())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *usermanagerSuite) TestEnableUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	user := names.NewUserTag("foobar")
	args := params.Entities{Entities: []params.Entity{{Tag: user.String()}}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: make([]params.ErrorResult, 1)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "EnableUser", args, result).SetArg(3, results).Return(nil)
	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.EnableUser(c.Context(), user.Name())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *usermanagerSuite) TestCantRemoveAdminUser(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	admin := names.NewUserTag("admin")
	args := params.Entities{
		Entities: []params.Entity{{Tag: "user-admin"}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{
		Results: []params.ErrorResult{{Error: &params.Error{Message: "failed to disable user: cannot disable controller model owner"}}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "DisableUser", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.DisableUser(c.Context(), admin.Name())
	c.Assert(err, tc.ErrorMatches, "failed to disable user: cannot disable controller model owner")
}

func (s *usermanagerSuite) TestUserInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	admin := names.NewUserTag("admin")
	args := params.UserInfoRequest{
		Entities:        []params.Entity{{Tag: "user-foobar"}},
		IncludeDisabled: true,
	}
	result := new(params.UserInfoResults)
	results := params.UserInfoResults{
		Results: []params.UserInfoResult{
			{
				Result: &params.UserInfo{
					Access:      "login",
					Username:    "foobar",
					DisplayName: "Foo Bar",
					CreatedBy:   admin.Name(),
				},
			},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UserInfo", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	obtained, err := client.UserInfo(c.Context(), []string{"foobar"}, usermanager.AllUsers)
	c.Assert(err, tc.ErrorIsNil)
	expected := []params.UserInfo{
		{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Access:      "login",
			CreatedBy:   "admin",
		},
	}

	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *usermanagerSuite) TestUserInfoMoreThanOneResult(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UserInfoRequest{
		IncludeDisabled: true,
	}
	result := new(params.UserInfoResults)
	results := params.UserInfoResults{
		Results: []params.UserInfoResult{
			{Result: &params.UserInfo{Username: "first"}},
			{Result: &params.UserInfo{Username: "second"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UserInfo", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	obtained, err := client.UserInfo(c.Context(), nil, usermanager.AllUsers)
	c.Assert(err, tc.ErrorIsNil)

	expected := []params.UserInfo{
		{Username: "first"},
		{Username: "second"},
	}

	c.Assert(obtained, tc.DeepEquals, expected)
}

func (s *usermanagerSuite) TestUserInfoMoreThanOneError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.UserInfoRequest{
		Entities:        []params.Entity{{Tag: "user-foo"}, {Tag: "user-bar"}},
		IncludeDisabled: true,
	}
	result := new(params.UserInfoResults)
	results := params.UserInfoResults{
		Results: []params.UserInfoResult{
			{Error: &params.Error{Message: "first error"}},
			{Error: &params.Error{Message: "second error"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "UserInfo", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.UserInfo(c.Context(), []string{"foo", "bar"}, usermanager.AllUsers)
	c.Assert(err, tc.ErrorMatches, "foo: first error, bar: second error")
}

func (s *usermanagerSuite) TestModelUserInfo(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d").String()}},
	}
	result := new(params.ModelUserInfoResults)
	results := params.ModelUserInfoResults{
		Results: []params.ModelUserInfoResult{
			{Result: &params.ModelUserInfo{UserName: "one"}},
			{Result: &params.ModelUserInfo{UserName: "two"}},
			{Result: &params.ModelUserInfo{UserName: "three"}},
		},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ModelUserInfo", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	obtained, err := client.ModelUserInfo(c.Context(), "deadbeef-0bad-400d-8000-4b1d0d06f00d")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.DeepEquals, []params.ModelUserInfo{
		{UserName: "one"},
		{UserName: "two"},
		{UserName: "three"},
	})
}

func (s *usermanagerSuite) TestSetUserPassword(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewUserTag("admin")
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{Tag: tag.String(), Password: "new-password"}},
	}
	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: make([]params.ErrorResult, 1)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetPassword", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.SetPassword(c.Context(), tag.Name(), "new-password")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *usermanagerSuite) TestSetUserPasswordCanonical(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewUserTag("admin")
	args := params.EntityPasswords{Changes: []params.EntityPassword{{Tag: tag.String(), Password: "new-password"}}}
	result := new(params.ErrorResults)
	results := params.ErrorResults{Results: make([]params.ErrorResult, 1)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "SetPassword", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.SetPassword(c.Context(), tag.Id(), "new-password")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *usermanagerSuite) TestSetUserPasswordBadName(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	err := client.SetPassword(c.Context(), "not!good", "new-password")
	c.Assert(err, tc.ErrorMatches, `"not!good" is not a valid username`)
}

func (s *usermanagerSuite) TestResetPasswordResponseError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("foobar").String()}},
	}
	result := new(params.AddUserResults)
	results := params.AddUserResults{Results: []params.AddUserResult{{Error: &params.Error{Message: "boom"}}}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResetPassword", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ResetPassword(c.Context(), "foobar")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *usermanagerSuite) TestResetPassword(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	key := []byte("no cats or dragons here")
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUserTag("foobar").String()}},
	}
	result := new(params.AddUserResults)
	results := params.AddUserResults{Results: []params.AddUserResult{{Tag: "user-foobar", SecretKey: key}}}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResetPassword", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	res, err := client.ResetPassword(c.Context(), "foobar")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, key)
}

func (s *usermanagerSuite) TestResetPasswordInvalidUsername(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ResetPassword(c.Context(), "not/valid")
	c.Assert(err, tc.ErrorMatches, `invalid user name "not/valid"`)
}

func (s *usermanagerSuite) TestResetPasswordResultCount(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := params.Entities{Entities: []params.Entity{{Tag: names.NewUserTag("foobar").String()}}}
	result := new(params.AddUserResults)
	results := params.AddUserResults{Results: make([]params.AddUserResult, 2)}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResetPassword", args, result).SetArg(3, results).Return(nil)

	client := usermanager.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ResetPassword(c.Context(), "foobar")
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}
