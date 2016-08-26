// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/testing"
)

type PermissionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&PermissionSuite{})

type fakeUserAccess struct {
	subjects []names.UserTag
	objects  []names.Tag
	user     description.UserAccess
	err      error
}

func (f *fakeUserAccess) call(subject names.UserTag, object names.Tag) (description.UserAccess, error) {
	f.subjects = append(f.subjects, subject)
	f.objects = append(f.objects, object)
	return f.user, f.err
}

func (r *PermissionSuite) TestNoUserTagLacksPermission(c *gc.C) {
	nonUser := names.NewModelTag("beef1beef1-0000-0000-000011112222")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	hasPermission, err := common.HasPermission((&fakeUserAccess{}).call, nonUser, description.ReadAccess, target)
	c.Assert(hasPermission, jc.IsFalse)
	c.Assert(err, jc.ErrorIsNil)
}

func (r *PermissionSuite) TestHasPermission(c *gc.C) {
	testCases := []struct {
		title            string
		userGetterAccess description.Access
		user             names.UserTag
		target           names.Tag
		access           description.Access
		expected         bool
	}{
		{
			title:            "user has lesser permissions than required",
			userGetterAccess: description.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           description.WriteAccess,
			expected:         false,
		},
		{
			title:            "user has equal permission than required",
			userGetterAccess: description.WriteAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           description.WriteAccess,
			expected:         true,
		},
		{
			title:            "user has greater permission than required",
			userGetterAccess: description.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           description.WriteAccess,
			expected:         true,
		},
		{
			title:            "user requests model permission on controller",
			userGetterAccess: description.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AddModelAccess,
			expected:         false,
		},
		{
			title:            "user requests controller permission on model",
			userGetterAccess: description.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AdminAccess, // notice user has this permission for model.
			expected:         false,
		},
		{
			title:            "controller permissions also work",
			userGetterAccess: description.AddModelAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AddModelAccess,
			expected:         true,
		},
	}
	for i, t := range testCases {
		userGetter := &fakeUserAccess{
			user: description.UserAccess{
				Access: t.userGetterAccess,
			}}
		c.Logf("HasPermission test n %d: %s", i, t.title)
		hasPermission, err := common.HasPermission(userGetter.call, t.user, t.access, t.target)
		c.Assert(hasPermission, gc.Equals, t.expected)
		c.Assert(err, jc.ErrorIsNil)
	}

}

func (r *PermissionSuite) TestUserGetterErrorReturns(c *gc.C) {
	user := names.NewUserTag("validuser")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	userGetter := &fakeUserAccess{
		user: description.UserAccess{},
		err:  errors.NotFoundf("a user"),
	}
	hasPermission, err := common.HasPermission(userGetter.call, user, description.ReadAccess, target)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasPermission, jc.IsFalse)
	c.Assert(userGetter.subjects, gc.HasLen, 1)
	c.Assert(userGetter.subjects[0], gc.DeepEquals, user)
	c.Assert(userGetter.objects, gc.HasLen, 1)
	c.Assert(userGetter.objects[0], gc.DeepEquals, target)
}

type fakeEveryoneUserAccess struct {
	user     description.UserAccess
	everyone description.UserAccess
}

func (f *fakeEveryoneUserAccess) call(subject names.UserTag, object names.Tag) (description.UserAccess, error) {
	if subject.Canonical() == common.EveryoneTagName {
		return f.everyone, nil
	}
	return f.user, nil
}

func (r *PermissionSuite) TestEveryoneAtExternal(c *gc.C) {
	testCases := []struct {
		title            string
		userGetterAccess description.Access
		everyoneAccess   description.Access
		user             names.UserTag
		target           names.Tag
		access           description.Access
		expected         bool
	}{
		{
			title:            "user has lesser permissions than everyone",
			userGetterAccess: description.LoginAccess,
			everyoneAccess:   description.AddModelAccess,
			user:             names.NewUserTag("validuser@external"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AddModelAccess,
			expected:         true,
		},
		{
			title:            "user has greater permissions than everyone",
			userGetterAccess: description.AddModelAccess,
			everyoneAccess:   description.LoginAccess,
			user:             names.NewUserTag("validuser@external"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AddModelAccess,
			expected:         true,
		},
		{
			title:            "everibody not considered if user is local",
			userGetterAccess: description.LoginAccess,
			everyoneAccess:   description.AddModelAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           description.AddModelAccess,
			expected:         false,
		},
	}

	for i, t := range testCases {
		userGetter := &fakeEveryoneUserAccess{
			user: description.UserAccess{
				Access: t.userGetterAccess,
			},
			everyone: description.UserAccess{
				Access: t.everyoneAccess,
			},
		}
		c.Logf(`HasPermission "everyone" test n %d: %s`, i, t.title)
		hasPermission, err := common.HasPermission(userGetter.call, t.user, t.access, t.target)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(hasPermission, gc.Equals, t.expected)
	}
}
