// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type EnvUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvUserSuite{})

func (s *EnvUserSuite) TestAddEnvironmentUser(c *gc.C) {
	now := state.NowToTheSecond()
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	envUser, err := s.State.AddEnvironmentUser(user.UserTag(), createdBy.UserTag(), "display-name")
	c.Assert(err, gc.IsNil)

	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:validusername@local", s.envTag.Id()))
	c.Assert(envUser.EnvironmentTag(), gc.Equals, s.envTag)
	c.Assert(envUser.UserName(), gc.Equals, "validusername@local")
	c.Assert(envUser.DisplayName(), gc.Equals, "display-name")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby@local")
	c.Assert(envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)

	envUser, err = s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, gc.IsNil)
	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:validusername@local", s.envTag.Id()))
	c.Assert(envUser.EnvironmentTag(), gc.Equals, s.envTag)
	c.Assert(envUser.UserName(), gc.Equals, "validusername@local")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby@local")
	c.Assert(envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *EnvUserSuite) TestAddEnvironmentNoUserFails(c *gc.C) {
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	_, err := s.State.AddEnvironmentUser(names.NewUserTag("validusername"), createdBy.UserTag(), "display-name")
	c.Assert(err, gc.ErrorMatches, `user "validusername" does not exist locally: user "validusername" not found`)
}

func (s *EnvUserSuite) TestAddEnvironmentNoCreatedByUserFails(c *gc.C) {
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	_, err := s.State.AddEnvironmentUser(user.UserTag(), names.NewUserTag("createdby"), user.DisplayName())
	c.Assert(err, gc.ErrorMatches, `createdBy user "createdby" does not exist locally: user "createdby" not found`)
}

func (s *EnvUserSuite) TestUpdateLastConnection(c *gc.C) {
	now := state.NowToTheSecond()
	createdBy := s.factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	user := s.factory.MakeUser(c, &factory.UserParams{Name: "validusername"})
	envUser, err := s.State.AddEnvironmentUser(user.UserTag(), createdBy.UserTag(), user.DisplayName())
	c.Assert(err, gc.IsNil)
	err = envUser.UpdateLastConnection()
	c.Assert(err, gc.IsNil)
	// It is possible that the update is done over a second boundary, so we need
	// to check for after now as well as equal.
	c.Assert(envUser.LastConnection().After(now) ||
		envUser.LastConnection().Equal(now), jc.IsTrue)
}
