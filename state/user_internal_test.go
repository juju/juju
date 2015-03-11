// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"
)

type internalUserSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&internalUserSuite{})

func (s *internalUserSuite) TestCreateInitialUserOp(c *gc.C) {
	tag := names.NewUserTag("AdMiN")
	op := createInitialUserOp(nil, tag, "abc")
	c.Assert(op.Id, gc.Equals, "admin")

	doc := op.Insert.(*userDoc)
	c.Assert(doc.DocID, gc.Equals, "admin")
	c.Assert(doc.Name, gc.Equals, "AdMiN")
}

func (s *internalUserSuite) TestCaseNameVsId(c *gc.C) {
	user, err := s.state.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(err, gc.IsNil)
	c.Assert(user.Name(), gc.Equals, "boB")
	c.Assert(user.doc.DocID, gc.Equals, "bob")
}
