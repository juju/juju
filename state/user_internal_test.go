// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/testing"
	"github.com/juju/names/v4"
)

type internalUserSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&internalUserSuite{})

func (s *internalUserSuite) TestCreateInitialUserOps(c *gc.C) {
	tag := names.NewUserTag("AdMiN")
	ops := createInitialUserOps(s.state.ControllerUUID(), tag, "abc", "salt", testing.ZeroTime())
	c.Assert(ops, gc.HasLen, 3)
	op := ops[0]
	c.Assert(op.Id, gc.Equals, "admin")

	doc := op.Insert.(*userDoc)
	c.Assert(doc.DocID, gc.Equals, "admin")
	c.Assert(doc.Name, gc.Equals, "AdMiN")
	c.Assert(doc.PasswordSalt, gc.Equals, "salt")

	// controller user permissions
	op = ops[1]
	permdoc := op.Insert.(*permissionDoc)
	c.Assert(permdoc.Access, gc.Equals, string(permission.SuperuserAccess))
	c.Assert(permdoc.ID, gc.Equals, permissionID(controllerKey(s.state.ControllerUUID()), userGlobalKey(strings.ToLower(tag.Id()))))
	c.Assert(permdoc.SubjectGlobalKey, gc.Equals, userGlobalKey(strings.ToLower(tag.Id())))
	c.Assert(permdoc.ObjectGlobalKey, gc.Equals, controllerKey(s.state.ControllerUUID()))

	// controller user
	op = ops[2]
	cudoc := op.Insert.(*userAccessDoc)
	c.Assert(cudoc.ID, gc.Equals, "admin")
	c.Assert(cudoc.ObjectUUID, gc.Equals, s.state.ControllerUUID())
	c.Assert(cudoc.UserName, gc.Equals, "AdMiN")
	c.Assert(cudoc.DisplayName, gc.Equals, "AdMiN")
	c.Assert(cudoc.CreatedBy, gc.Equals, "AdMiN")
}

func (s *internalUserSuite) TestCaseNameVsId(c *gc.C) {
	user, err := s.state.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(err, gc.IsNil)
	c.Assert(user.Name(), gc.Equals, "boB")
	c.Assert(user.doc.DocID, gc.Equals, "bob")
}
