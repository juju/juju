// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/lease"
)

// ClientValidationSuite sends bad data into all of Client's methods.
type ClientValidationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientValidationSuite{})

func (s *ClientValidationSuite) TestNewClientId(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Id = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid id: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientNamespace(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Namespace = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid namespace: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientCollection(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Collection = "$bad"
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid collection: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestNewClientMongo(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Mongo = nil
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing mongo")
}

func (s *ClientValidationSuite) TestNewClientClock(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Clock = nil
	_, err := lease.NewClient(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing clock")
}

func (s *ClientValidationSuite) TestClaimLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease("$name", lease.Request{"holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestClaimLeaseHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease("name", lease.Request{"$holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestClaimLeaseDuration(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease("name", lease.Request{"holder", 0})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *ClientValidationSuite) TestExtendLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ExtendLease("$name", lease.Request{"holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestExtendLeaseHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ExtendLease("name", lease.Request{"$holder", time.Minute})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *ClientValidationSuite) TestExtendLeaseDuration(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ExtendLease("name", lease.Request{"holder", 0})
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *ClientValidationSuite) TestExpireLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ExpireLease("$name")
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}
