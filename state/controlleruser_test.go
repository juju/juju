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
	controllerUser, err := s.State.ControllerUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access(), gc.Equals, description.LoginAccess)
}

func (s *ControllerUserSuite) TestSetAccessControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c,
		&factory.UserParams{
			Name: "validusername",
		})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	t := user.Tag()
	userTag := t.(names.UserTag)
	controllerUser, err := s.State.ControllerUser(userTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerUser.Access(), gc.Equals, description.LoginAccess)

	controllerUser.SetAccess(description.AddModelAccess)

	controllerUser, err = s.State.ControllerUser(user.UserTag())
	c.Assert(controllerUser.Access(), gc.Equals, description.AddModelAccess)
}

//---------
func (s *ControllerUserSuite) TestRemoveControllerUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername"})
	_, err := s.State.ControllerUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveControllerUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.ControllerUser(user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

// TestCaseSensitiveControllerUserErrors tests that the user id is not case sensitive
// and adding two times the same username with different capitalization will
// fail, even though it is actually testing the mechanism for AddUser (which fails
// before controller user adding) the test is here in case anyone changes the
// logic behind Adding regular users and forgets to do the same for ControllerUsers.
func (s *ControllerUserSuite) TestCaseSensitiveControllerUserErrors(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "VALIDuSERNAME"})
	_, err := s.State.ControllerUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)

	user = s.Factory.MakeUser(c, &factory.UserParams{Name: "validUsername",
		ExpectedCreateError: `user already exists`,
		NoModelUser:         true})
}

func (s *ControllerUserSuite) TestRemoveControllerUserSucceeds(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	err := s.State.RemoveControllerUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ControllerUserSuite) TestRemoveControllerUserFails(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{})
	err := s.State.RemoveControllerUser(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.RemoveControllerUser(user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
