// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission_test

import (
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
)

type accessSuite struct{}

var _ = gc.Suite(&accessSuite{})

func (*accessSuite) TestEqualOrGreaterModelAccessThan(c *gc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the controller permissions return true for any comparison.
	for _, value := range []permission.Access{login, addmodel, superuser} {
		c.Check(value.EqualOrGreaterModelAccessThan(undefined), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(read), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(write), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(admin), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(login), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.EqualOrGreaterModelAccessThan(login), jc.IsFalse)
		c.Check(value.EqualOrGreaterModelAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.EqualOrGreaterModelAccessThan(addmodel), jc.IsFalse)
	}

	c.Check(undefined.EqualOrGreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(undefined.EqualOrGreaterModelAccessThan(read), jc.IsFalse)
	c.Check(undefined.EqualOrGreaterModelAccessThan(write), jc.IsFalse)
	c.Check(undefined.EqualOrGreaterModelAccessThan(admin), jc.IsFalse)

	c.Check(read.EqualOrGreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(read.EqualOrGreaterModelAccessThan(read), jc.IsTrue)
	c.Check(read.EqualOrGreaterModelAccessThan(write), jc.IsFalse)
	c.Check(read.EqualOrGreaterModelAccessThan(admin), jc.IsFalse)

	c.Check(write.EqualOrGreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(read), jc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(write), jc.IsTrue)
	c.Check(write.EqualOrGreaterModelAccessThan(admin), jc.IsFalse)

	c.Check(admin.EqualOrGreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(read), jc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(write), jc.IsTrue)
	c.Check(admin.EqualOrGreaterModelAccessThan(admin), jc.IsTrue)
}

func (*accessSuite) TestGreaterModelAccessThan(c *gc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of undefined or the controller permissions return true for any comparison.
	for _, value := range []permission.Access{undefined, login, addmodel, superuser} {
		c.Check(value.GreaterModelAccessThan(undefined), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(read), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(write), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(admin), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(login), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterModelAccessThan(login), jc.IsFalse)
		c.Check(value.GreaterModelAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterModelAccessThan(addmodel), jc.IsFalse)
	}

	c.Check(read.GreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(read.GreaterModelAccessThan(read), jc.IsFalse)
	c.Check(read.GreaterModelAccessThan(write), jc.IsFalse)
	c.Check(read.GreaterModelAccessThan(admin), jc.IsFalse)

	c.Check(write.GreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(write.GreaterModelAccessThan(read), jc.IsTrue)
	c.Check(write.GreaterModelAccessThan(write), jc.IsFalse)
	c.Check(write.GreaterModelAccessThan(admin), jc.IsFalse)

	c.Check(admin.GreaterModelAccessThan(undefined), jc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(read), jc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(write), jc.IsTrue)
	c.Check(admin.GreaterModelAccessThan(admin), jc.IsFalse)
}

func (*accessSuite) TestEqualOrGreaterControllerAccessThan(c *gc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the model permissions return true for any comparison.
	for _, value := range []permission.Access{read, write, admin} {
		c.Check(value.EqualOrGreaterControllerAccessThan(undefined), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(read), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(admin), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(login), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(read), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(admin), jc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), jc.IsFalse)
	}

	c.Check(undefined.EqualOrGreaterControllerAccessThan(undefined), jc.IsTrue)
	c.Check(undefined.EqualOrGreaterControllerAccessThan(login), jc.IsFalse)
	c.Check(undefined.EqualOrGreaterControllerAccessThan(superuser), jc.IsFalse)

	c.Check(login.EqualOrGreaterControllerAccessThan(undefined), jc.IsTrue)
	c.Check(login.EqualOrGreaterControllerAccessThan(login), jc.IsTrue)
	c.Check(login.EqualOrGreaterControllerAccessThan(superuser), jc.IsFalse)

	c.Check(superuser.EqualOrGreaterControllerAccessThan(undefined), jc.IsTrue)
	c.Check(superuser.EqualOrGreaterControllerAccessThan(login), jc.IsTrue)
	c.Check(superuser.EqualOrGreaterControllerAccessThan(superuser), jc.IsTrue)
}

func (*accessSuite) TestGreaterControllerAccessThan(c *gc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		undefined = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of undefined or the model permissions return true for any comparison.
	for _, value := range []permission.Access{undefined, read, write, admin} {
		c.Check(value.GreaterControllerAccessThan(undefined), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(read), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(write), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(admin), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(login), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(superuser), jc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.GreaterControllerAccessThan(read), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(write), jc.IsFalse)
		c.Check(value.GreaterControllerAccessThan(admin), jc.IsFalse)
	}
	// No comparison against a cloud permission will return true
	for _, value := range []permission.Access{undefined, login, superuser} {
		c.Check(value.GreaterModelAccessThan(addmodel), jc.IsFalse)
	}

	c.Check(login.GreaterControllerAccessThan(undefined), jc.IsTrue)
	c.Check(login.GreaterControllerAccessThan(login), jc.IsFalse)
	c.Check(login.GreaterControllerAccessThan(superuser), jc.IsFalse)

	c.Check(superuser.GreaterControllerAccessThan(undefined), jc.IsTrue)
	c.Check(superuser.GreaterControllerAccessThan(login), jc.IsTrue)
	c.Check(superuser.GreaterControllerAccessThan(superuser), jc.IsFalse)
}

func (*accessSuite) TestEqualOrGreaterCloudAccessThan(c *gc.C) {
	// A very boring but necessary test to test explicit responses.
	var (
		noaccess  = permission.NoAccess
		read      = permission.ReadAccess
		write     = permission.WriteAccess
		admin     = permission.AdminAccess
		login     = permission.LoginAccess
		addmodel  = permission.AddModelAccess
		superuser = permission.SuperuserAccess
	)
	// None of the model permissions return true for any comparison.
	for _, value := range []permission.Access{read, write} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), jc.IsFalse)
	}
	// No comparison against a model permission will return true
	for _, value := range []permission.Access{addmodel} {
		c.Check(value.EqualOrGreaterControllerAccessThan(read), jc.IsFalse)
		c.Check(value.EqualOrGreaterControllerAccessThan(write), jc.IsFalse)
	}
	// No comparison against a controller permission will return true
	for _, value := range []permission.Access{noaccess, login, superuser} {
		c.Check(value.EqualOrGreaterControllerAccessThan(addmodel), jc.IsFalse)
	}

	c.Check(noaccess.EqualOrGreaterCloudAccessThan(noaccess), jc.IsTrue)
	c.Check(noaccess.EqualOrGreaterCloudAccessThan(addmodel), jc.IsFalse)
	c.Check(noaccess.EqualOrGreaterCloudAccessThan(admin), jc.IsFalse)

	c.Check(addmodel.EqualOrGreaterCloudAccessThan(addmodel), jc.IsTrue)
	c.Check(addmodel.EqualOrGreaterCloudAccessThan(noaccess), jc.IsTrue)
	c.Check(addmodel.EqualOrGreaterCloudAccessThan(admin), jc.IsFalse)

	c.Check(admin.EqualOrGreaterCloudAccessThan(noaccess), jc.IsTrue)
	c.Check(admin.EqualOrGreaterCloudAccessThan(addmodel), jc.IsTrue)
	c.Check(admin.EqualOrGreaterCloudAccessThan(admin), jc.IsTrue)
}

var validateObjectTypeTest = []struct {
	access     permission.Access
	objectType permission.ObjectType
	fail       bool
}{
	{access: permission.AdminAccess, objectType: permission.Cloud},
	{access: permission.SuperuserAccess, objectType: permission.Cloud, fail: true},
	{access: permission.LoginAccess, objectType: permission.Controller},
	{access: permission.ReadAccess, objectType: permission.Controller, fail: true},
	{access: permission.ReadAccess, objectType: permission.Model},
	{access: permission.ConsumeAccess, objectType: permission.Model, fail: true},
	{access: permission.ConsumeAccess, objectType: permission.Offer},
	{access: permission.AddModelAccess, objectType: permission.Offer, fail: true},
	{access: permission.AddModelAccess, objectType: "failme", fail: true},
}

func (*accessSuite) TestValidateAccessForObjectType(c *gc.C) {
	size := len(validateObjectTypeTest)
	for i, test := range validateObjectTypeTest {
		c.Logf("Running test %d of %d", i, size)
		id := permission.ID{ObjectType: test.objectType}
		err := id.ValidateAccess(test.access)
		if test.fail {
			c.Assert(err, jc.ErrorIs, coreerrors.NotValid, gc.Commentf("test %d", i))
		} else {
			c.Check(err, jc.ErrorIsNil, gc.Commentf("test %d", i))
		}
	}
}

func (*accessSuite) TestParseTagForID(c *gc.C) {
	id, err := permission.ParseTagForID(names.NewCloudTag("testcloud"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(id.ObjectType, gc.Equals, permission.Cloud)
}

func (*accessSuite) TestParseTagForIDFail(c *gc.C) {
	_, err := permission.ParseTagForID(nil)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
	_, err = permission.ParseTagForID(names.NewUserTag("testcloud"))
	c.Assert(err, jc.ErrorIs, coreerrors.NotSupported)
}
