// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time" // Only used for time types.

	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/state/lease"
)

// StoreValidationSuite sends bad data into all of Store's methods.
type StoreValidationSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&StoreValidationSuite{})

func (s *StoreValidationSuite) TestNewStoreId(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Id = "$bad"
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid id: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestNewStoreNamespace(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Namespace = "$bad"
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid namespace: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestNewStoreCollection(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Collection = "$bad"
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "invalid collection: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestNewStoreMongo(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.Mongo = nil
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing mongo")
}

func (s *StoreValidationSuite) TestNewStoreLocalClock(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.LocalClock = nil
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing local clock")
}

func (s *StoreValidationSuite) TestNewStoreGlobalClock(c *gc.C) {
	fix := s.EasyFixture(c)
	fix.Config.GlobalClock = nil
	_, err := lease.NewStore(fix.Config)
	c.Check(err, gc.ErrorMatches, "missing global clock")
}

func (s *StoreValidationSuite) TestClaimLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("$name"), corelease.Request{"holder", time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestClaimLeaseHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), corelease.Request{"$holder", time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestClaimLeaseDuration(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), corelease.Request{"holder", 0}, nil)
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *StoreValidationSuite) TestExtendLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ExtendLease(key("$name"), corelease.Request{"holder", time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestExtendLeaseHolder(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ExtendLease(key("name"), corelease.Request{"$holder", time.Minute}, nil)
	c.Check(err, gc.ErrorMatches, "invalid request: invalid holder: string contains forbidden characters")
}

func (s *StoreValidationSuite) TestExtendLeaseDuration(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ExtendLease(key("name"), corelease.Request{"holder", 0}, nil)
	c.Check(err, gc.ErrorMatches, "invalid request: invalid duration")
}

func (s *StoreValidationSuite) TestExpireLeaseName(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ExpireLease(key("$name"))
	c.Check(err, gc.ErrorMatches, "invalid name: string contains forbidden characters")
}
