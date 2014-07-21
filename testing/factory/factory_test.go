// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type factorySuite struct {
	testing.BaseSuite
	jtesting.MgoSuite
	State   *state.State
	Factory *factory.Factory
}

var _ = gc.Suite(&factorySuite{})

func (s *factorySuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *factorySuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *factorySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	policy := statetesting.MockPolicy{}

	info := &authentication.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{jtesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}
	opts := mongo.DialOpts{
		Timeout: testing.LongWait,
	}
	cfg := testing.EnvironConfig(c)
	st, err := state.Initialize(info, cfg, opts, &policy)
	c.Assert(err, gc.IsNil)
	s.State = st
	s.Factory = factory.NewFactory(s.State, c)
}

func (s *factorySuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		s.State.Close()
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *factorySuite) TestMakeUserAny(c *gc.C) {
	user := s.Factory.MakeAnyUser()
	c.Assert(user.IsDeactivated(), jc.IsFalse)

	saved, err := s.State.User(user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.LastConnection(), gc.Equals, user.LastConnection())
	c.Assert(saved.IsDeactivated(), gc.Equals, user.IsDeactivated())
}

func (s *factorySuite) TestMakeUserParams(c *gc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := "eric"
	password := "sekrit"
	user := s.Factory.MakeUser(factory.UserParams{
		Username:    username,
		DisplayName: displayName,
		Creator:     creator,
		Password:    password,
	})
	c.Assert(user.IsDeactivated(), jc.IsFalse)
	c.Assert(user.Name(), gc.Equals, username)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.PasswordValid(password), jc.IsTrue)

	saved, err := s.State.User(user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.LastConnection(), gc.Equals, user.LastConnection())
	c.Assert(saved.IsDeactivated(), gc.Equals, user.IsDeactivated())
}
