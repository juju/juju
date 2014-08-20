// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type EnvUserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&EnvUserSuite{})

func (s *EnvUserSuite) TestAddInvalidTags(c *gc.C) {
	for _, name := range []string{
		"",
		"a",
		"b^b",
		"a.",
		"a-",
	} {
		c.Logf("check invalid name %q", name)
		envUser, err := s.State.AddEnvironmentUser(name, "ignored", "ignored", "ignored")
		c.Check(err, gc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Check(envUser, gc.IsNil)
	}
}

func (s *EnvUserSuite) TestAddEnvironmentUser(c *gc.C) {
	now := state.NowToTheSecond()
	fac := factory.NewFactory(s.State, c)
	envUuid := fac.EnvironTag().String()
	envUser, err := s.State.AddEnvironmentUser("user-valid", "display-name", "alias", "createdby")
	c.Assert(err, gc.IsNil)

	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:user-valid", envUuid))
	c.Assert(envUser.EnvUUID(), gc.Equals, envUuid)
	c.Assert(envUser.UserName(), gc.Equals, "user-valid")
	c.Assert(envUser.Alias(), gc.Equals, "alias")
	c.Assert(envUser.DisplayName(), gc.Equals, "display-name")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby")
	c.Assert(envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)

	envUser, err = s.State.EnvironmentUser("user-valid")
	c.Assert(err, gc.IsNil)
	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:user-valid", envUuid))
	c.Assert(envUser.EnvUUID(), gc.Equals, envUuid)
	c.Assert(envUser.UserName(), gc.Equals, "user-valid")
	c.Assert(envUser.Alias(), gc.Equals, "alias")
	c.Assert(envUser.DisplayName(), gc.Equals, "display-name")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby")
	c.Assert(envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *UserSuite) TestUpdateLastConnection(c *gc.C) {
	now := state.NowToTheSecond()
	envUser, err := s.State.AddEnvironmentUser("user-valid", "display-name", "alias", "createdby")
	c.Assert(err, gc.IsNil)
	err = envUser.UpdateLastConnection()
	c.Assert(err, gc.IsNil)
	c.Assert(envUser.LastConnection().Equal(now), jc.IsTrue)
}
