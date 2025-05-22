// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/testing"
)

type PermissionSuite struct {
	testing.BaseSuite
}

func TestPermissionSuite(t *stdtesting.T) {
	tc.Run(t, &PermissionSuite{})
}

type fakeUserAccess struct {
	userNames []user.Name
	targets   []permission.ID
	access    permission.Access
	err       error
}

func (f *fakeUserAccess) call(ctx context.Context, userName user.Name, target permission.ID) (permission.Access, error) {
	f.userNames = append(f.userNames, userName)
	f.targets = append(f.targets, target)
	return f.access, f.err
}

func (r *PermissionSuite) TestNoUserTagLacksPermission(c *tc.C) {
	nonUser := names.NewModelTag("beef1beef1-0000-0000-000011112222")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	hasPermission, err := common.HasPermission(c.Context(), (&fakeUserAccess{}).call, nonUser, permission.ReadAccess, target)
	c.Assert(hasPermission, tc.IsFalse)
	c.Assert(err, tc.ErrorIsNil)
}

func (r *PermissionSuite) TestHasPermission(c *tc.C) {
	testCases := []struct {
		title            string
		userGetterAccess permission.Access
		user             names.UserTag
		target           names.Tag
		access           permission.Access
		expected         bool
	}{
		{
			title:            "user has lesser permissions than required",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         false,
		},
		{
			title:            "user has equal permission than required",
			userGetterAccess: permission.WriteAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         true,
		},
		{
			title:            "user has greater permission than required",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.WriteAccess,
			expected:         true,
		},
		{
			title:            "user requests model permission on controller",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewModelTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
		{
			title:            "user requests controller permission on model",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.AdminAccess, // notice user has this permission for model.
			expected:         false,
		},
		{
			title:            "controller permissions also work",
			userGetterAccess: permission.SuperuserAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewControllerTag("beef1beef2-0000-0000-000011112222"),
			access:           permission.SuperuserAccess,
			expected:         true,
		},
		{
			title:            "cloud permissions work",
			userGetterAccess: permission.AddModelAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewCloudTag("mycloud"),
			access:           permission.AddModelAccess,
			expected:         true,
		},
		{
			title:            "user has lesser cloud permissions than required",
			userGetterAccess: permission.NoAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewCloudTag("mycloud"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
		{
			title:            "user has lesser offer permissions than required",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.WriteAccess,
			expected:         false,
		},
		{
			title:            "user has equal offer permission than required",
			userGetterAccess: permission.ConsumeAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.ConsumeAccess,
			expected:         true,
		},
		{
			title:            "user has greater offer permission than required",
			userGetterAccess: permission.AdminAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.ConsumeAccess,
			expected:         true,
		},
		{
			title:            "user requests controller permission on offer",
			userGetterAccess: permission.ReadAccess,
			user:             names.NewUserTag("validuser"),
			target:           names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			access:           permission.AddModelAccess,
			expected:         false,
		},
	}
	for i, t := range testCases {
		userGetter := &fakeUserAccess{
			access: t.userGetterAccess,
		}
		c.Logf("HasPermission test n %d: %s", i, t.title)
		hasPermission, err := common.HasPermission(c.Context(), userGetter.call, t.user, t.access, t.target)
		c.Assert(hasPermission, tc.Equals, t.expected)
		c.Assert(err, tc.ErrorIsNil)
	}

}

func (r *PermissionSuite) TestUserGetterErrorReturns(c *tc.C) {
	userTag := names.NewUserTag("validuser")
	target := names.NewModelTag("beef1beef2-0000-0000-000011112222")
	userGetter := &fakeUserAccess{
		access: permission.NoAccess,
		err:    accesserrors.AccessNotFound,
	}
	hasPermission, err := common.HasPermission(c.Context(), userGetter.call, userTag, permission.ReadAccess, target)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(hasPermission, tc.IsFalse)
	c.Assert(userGetter.userNames, tc.HasLen, 1)
	c.Assert(userGetter.userNames[0], tc.DeepEquals, user.NameFromTag(userTag))
	c.Assert(userGetter.targets, tc.HasLen, 1)
	c.Assert(userGetter.targets[0], tc.DeepEquals, permission.ID{ObjectType: permission.Model, Key: target.Id()})
}
