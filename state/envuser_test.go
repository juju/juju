// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
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
		envUser, err := s.State.AddEnvUser("ignored", name, "ignored", "ignored", "ignored")
		c.Check(err, gc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Check(envUser, gc.IsNil)
	}

	for _, env := range []string{
		"env/foo",
	} {
		c.Logf("check invalid environment %q", env)
		envUser, err := s.State.AddEnvUser(env, "user-valid", "ignored", "ignored", "ignored")
		c.Check(err, gc.ErrorMatches, `invalid environment "`+regexp.QuoteMeta(env)+`"`)
		c.Check(envUser, gc.IsNil)
	}
}

func (s *EnvUserSuite) TestAddEnvUser(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	fac := factory.NewFactory(s.State, c)
	envUuid := fac.NewUUID()
	envUser, err := s.State.AddEnvUser(envUuid, "user-valid", "display-name", "alias", "createdby")
	c.Assert(err, gc.IsNil)

	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:user-valid", envUuid))
	c.Assert(envUser.EnvUUID(), gc.Equals, envUuid)
	c.Assert(envUser.UserName(), gc.Equals, "user-valid")
	c.Assert(envUser.Alias(), gc.Equals, "alias")
	c.Assert(envUser.DisplayName(), gc.Equals, "display-name")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby")
	c.Assert(envUser.DateCreated().After(now) || envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)

	envUser, err = s.State.EnvUser(envUuid, "user-valid")
	c.Assert(err, gc.IsNil)
	c.Assert(envUser.ID(), gc.Equals, fmt.Sprintf("%s:user-valid", envUuid))
	c.Assert(envUser.EnvUUID(), gc.Equals, envUuid)
	c.Assert(envUser.UserName(), gc.Equals, "user-valid")
	c.Assert(envUser.Alias(), gc.Equals, "alias")
	c.Assert(envUser.DisplayName(), gc.Equals, "display-name")
	c.Assert(envUser.CreatedBy(), gc.Equals, "createdby")
	c.Assert(envUser.DateCreated().After(now) || envUser.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(envUser.LastConnection(), gc.IsNil)
}

func (s *UserSuite) TestUpdateLastConnection(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	fac := factory.NewFactory(s.State, c)
	envUuid := fac.NewUUID()
	envUser, err := s.State.AddEnvUser(envUuid, "user-valid", "display-name", "alias", "createdby")
	c.Assert(err, gc.IsNil)
	err = envUser.UpdateLastConnection()
	c.Assert(err, gc.IsNil)
	c.Assert(envUser.LastConnection().After(now) ||
		envUser.LastConnection().Equal(now), jc.IsTrue)
}
