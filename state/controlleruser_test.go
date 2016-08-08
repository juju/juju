// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/testing/factory"
)

type ControllerUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ControllerUserSuite{})

type accessAwareUser interface {
	Access() description.Access
}

func (s *ControllerUserSuite) TestDefaultAccessControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	t := user.Tag()
	userTag := t.(names.UserTag)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.LoginAccess)
}

func (s *ControllerUserSuite) TestSetAccessControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	t := user.Tag()
	userTag := t.(names.UserTag)
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	controllerUser, err := s.State.UserAccess(userTag, ctag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access, gc.Equals, description.LoginAccess)

	s.State.SetUserAccess(userTag, ctag, description.AddModelAccess)

	controllerUser, err = s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(controllerUser.Access, gc.Equals, description.AddModelAccess)
}

func (s *ControllerUserSuite) TestRemoveControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	_, err := s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.UserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *ControllerUserSuite) TestRemoveControllerUserSucceeds(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerUserSuite) TestRemoveControllerUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	ctag := names.NewControllerTag(s.State.ControllerUUID())
	err := s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveUserAccess(user.UserTag(), ctag)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
