// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	corelease "github.com/juju/juju/core/lease"
	statelease "github.com/juju/juju/state/lease"
)

// ClientAssertSuite tests that AssertOp does what it should.
type ClientSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestLookupLeaseNotThere(c *gc.C) {
	db := NewMongo(s.db)
	coll, closer := db.GetCollection("default-collection")
	defer closer()
	_, err := statelease.LookupLease(coll, "default-namespace", "bar")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *ClientSuite) TestLookupLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Client.ClaimLease("name", corelease.Request{"holder", time.Minute})
	c.Assert(err, jc.ErrorIsNil)
	db := NewMongo(s.db)
	coll, closer := db.GetCollection("default-collection")
	defer closer()
	doc, err := statelease.LookupLease(coll, "default-namespace", "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(doc.Name, gc.Equals, "name")
	c.Check(doc.Holder, gc.Equals, "holder")
	c.Check(doc.Namespace, gc.Equals, "default-namespace")
}
