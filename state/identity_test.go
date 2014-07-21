// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type IdentitySuite struct {
	ConnSuite
}

var _ = gc.Suite(&IdentitySuite{})

func (s *IdentitySuite) TestAddInvalidNames(c *gc.C) {
	for _, name := range []string{
		"",
		"a",
		"b^b",
		"a.",
		"a-",
	} {
		identity, err := s.State.AddIdentity(name, "ignored", "ignored", "ignored")
		c.Assert(err, gc.ErrorMatches, `invalid identity name "`+regexp.QuoteMeta(name)+`"`)
		c.Assert(identity, gc.IsNil)
	}
}

func (s *IdentitySuite) TestAddIdentity(c *gc.C) {
	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"
	creator := "admin"

	now := time.Now().Round(time.Second).UTC()

	identity, err := s.State.AddIdentity(name, displayName, password, creator)
	c.Assert(err, gc.IsNil)
	c.Assert(identity, gc.NotNil)
	c.Assert(identity.Name(), gc.Equals, name)
	c.Assert(identity.DisplayName(), gc.Equals, displayName)
	c.Assert(identity.PasswordValid(password), jc.IsTrue)
	c.Assert(identity.CreatedBy(), gc.Equals, creator)
	c.Assert(identity.DateCreated().After(now) ||
		identity.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(identity.LastLogin(), gc.IsNil)

	identity, err = s.State.Identity(name)
	c.Assert(err, gc.IsNil)
	c.Check(identity, gc.NotNil)
	c.Assert(identity.Name(), gc.Equals, name)
	c.Assert(identity.DisplayName(), gc.Equals, displayName)
	c.Assert(identity.PasswordValid(password), jc.IsTrue)
	c.Assert(identity.CreatedBy(), gc.Equals, creator)
	c.Assert(identity.DateCreated().After(now) ||
		identity.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(identity.LastLogin(), gc.IsNil)
}

func (s *IdentitySuite) TestUpdateLastLogin(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	identity := s.factory.MakeAnyIdentity()
	err := identity.UpdateLastLogin()
	c.Assert(err, gc.IsNil)
	c.Assert(identity.LastLogin().After(now) ||
		identity.LastLogin().Equal(now), jc.IsTrue)
}

func (s *IdentitySuite) TestSetPassword(c *gc.C) {
	identity := s.factory.MakeAnyIdentity()
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.Identity(identity.Name())
	})
}

func (s *IdentitySuite) TestAddIdentitySetsSalt(c *gc.C) {
	identity := s.factory.MakeIdentity(factory.IdentityParams{Password: "a-password"})
	salt, hash := state.GetIdentityPasswordSaltAndHash(identity)
	c.Check(hash, gc.Not(gc.Equals), "")
	c.Check(salt, gc.Not(gc.Equals), "")
	c.Check(utils.UserPasswordHash("a-password", salt), gc.Equals, hash)
	c.Check(identity.PasswordValid("a-password"), jc.IsTrue)
}

func (s *IdentitySuite) TestSetPasswordChangesSalt(c *gc.C) {
	identity := s.factory.MakeAnyIdentity()
	origSalt, origHash := state.GetIdentityPasswordSaltAndHash(identity)
	c.Check(origSalt, gc.Not(gc.Equals), "")
	identity.SetPassword("a-password")
	newSalt, newHash := state.GetIdentityPasswordSaltAndHash(identity)
	c.Check(newSalt, gc.Not(gc.Equals), "")
	c.Check(newSalt, gc.Not(gc.Equals), origSalt)
	c.Check(newHash, gc.Not(gc.Equals), origHash)
	c.Check(identity.PasswordValid("a-password"), jc.IsTrue)
}

func (s *IdentitySuite) TestDeactivate(c *gc.C) {
	identity := s.factory.MakeAnyIdentity()
	c.Assert(identity.IsDeactivated(), gc.Equals, false)

	err := identity.Deactivate()
	c.Assert(err, gc.IsNil)
	c.Assert(identity.IsDeactivated(), gc.Equals, true)
	c.Assert(identity.PasswordValid(""), gc.Equals, false)
}

func (s *IdentitySuite) TestCantDeactivateAdmin(c *gc.C) {
	// TODO: when the ConnSuite is updated to create the admin identity for the
	// admin user, we can remove the creation here (in fact it should cause this
	// test to fail).
	s.factory.MakeIdentity(factory.IdentityParams{Username: state.AdminIdentity})

	identity, err := s.State.Identity(state.AdminIdentity)
	c.Assert(err, gc.IsNil)
	err = identity.Deactivate()
	c.Assert(err, gc.ErrorMatches, "can't deactivate admin identity")
}
