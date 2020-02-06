// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&controllerSuite{})

type controllerSuite struct{}

func (s *controllerSuite) TestUserListCompatibility(c *gc.C) {
	extProvider1 := "https://api.jujucharms.com/identity"
	extProvider2 := "http://candid.provider/identity"
	specs := []struct {
		descr    string
		src, dst userList
		expErr   string
	}{
		{
			descr: `all src users present in dst`,
			src: userList{
				users: set.NewStrings("foo", "bar"),
			},
			dst: userList{
				users: set.NewStrings("foo", "bar"),
			},
		},
		{
			descr: `local src users present in dst, and an external user has been granted access, and src/dst use the same identity provider url`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("foo"),
				identityURL: extProvider1,
			},
		},
		{
			descr: `some local src users not present in dst`,
			src: userList{
				users: set.NewStrings("foo", "bar"),
			},
			dst: userList{
				users: set.NewStrings("bar"),
			},
			expErr: `cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - foo`,
		},
		{
			descr: `local src users present in dst, and an external user has been granted access, and src/dst use different identity provider URL`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider2,
			},
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you can remove the following users from the current model:
  - bar@external`,
		},
		{
			descr: `not all local src users present in dst, and an external user has been granted access, and src/dst use different identity provider URL`,
			src: userList{
				users:       set.NewStrings("foo", "bar@external"),
				identityURL: extProvider1,
			},
			dst: userList{
				users:       set.NewStrings("baz", "bar@external"),
				identityURL: extProvider2,
			},
			expErr: `cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you need to remove the following users from the current model:
  - bar@external

and add the following users to the destination controller or remove them from
the current model:
  - foo`,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("test %d: %s", specIndex, spec.descr)

		err := spec.src.checkCompatibilityWith(spec.dst)
		if spec.expErr == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.Not(gc.Equals), nil)
			c.Assert(err.Error(), gc.Equals, spec.expErr)
		}
	}
}
