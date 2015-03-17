// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type internalEnvUserSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&internalEnvUserSuite{})

func (s *internalEnvUserSuite) TestCreateEnvUserOpAndDoc(c *gc.C) {
	tag := names.NewUserTag("UserName")
	op, doc := createEnvUserOpAndDoc("ignored", tag, names.NewUserTag("ignored"), "ignored")

	c.Assert(op.Id, gc.Equals, "username@local")
	c.Assert(doc.ID, gc.Equals, "username@local")
	c.Assert(doc.UserName, gc.Equals, "UserName@local")
}

func (s *internalEnvUserSuite) TestCaseUserNameVsId(c *gc.C) {
	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)

	user, err := s.state.AddEnvironmentUser(names.NewUserTag("Bob@RandomProvider"), env.Owner(), "")
	c.Assert(err, gc.IsNil)
	c.Assert(user.UserName(), gc.Equals, "Bob@RandomProvider")
	c.Assert(user.doc.ID, gc.Equals, s.state.docID("bob@randomprovider"))
}
