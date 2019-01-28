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

type StoreSuite struct {
	FixtureSuite
}

var _ = gc.Suite(&StoreSuite{})

func (s *StoreSuite) TestLookupLeaseNotThere(c *gc.C) {
	db := NewMongo(s.db)
	coll, closer := db.GetCollection("default-collection")
	defer closer()
	_, err := statelease.LookupLease(coll, "default-namespace", "bar")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *StoreSuite) TestLookupLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), corelease.Request{Holder: "holder", Duration: time.Minute})
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

func (s *StoreSuite) TestTrapdoorBadKeyErrors(c *gc.C) {
	fix := s.EasyFixture(c)

	_, err := fix.Store.Trapdoor(corelease.Key{Namespace: "nope"}, "")
	c.Assert(err, gc.ErrorMatches, `store namespace is "default-namespace", but lease requested for "nope"`)

	_, err = fix.Store.Trapdoor(corelease.Key{Namespace: defaultNamespace, ModelUUID: "nope"}, "")
	c.Assert(err, gc.ErrorMatches, `store model UUID is "model-uuid", but lease requested for "nope"`)
}

func (s *StoreSuite) TestTrapdoorNoLease(c *gc.C) {
	fix := s.EasyFixture(c)

	leaseKey := corelease.Key{
		Namespace: defaultNamespace,
		ModelUUID: "model-uuid",
		Lease:     "name",
	}
	_, err := fix.Store.Trapdoor(leaseKey, "holder")
	c.Assert(err, gc.Equals, corelease.ErrNotHeld)
}

func (s *StoreSuite) TestTrapdoorNotHolder(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("name"), corelease.Request{Holder: "holder", Duration: time.Minute})
	c.Assert(err, jc.ErrorIsNil)

	leaseKey := corelease.Key{
		Namespace: defaultNamespace,
		ModelUUID: "model-uuid",
		Lease:     "name",
	}
	_, err = fix.Store.Trapdoor(leaseKey, "nope")
	c.Assert(err, gc.Equals, corelease.ErrNotHeld)
}

func (s *StoreSuite) TestTrapdoorSuccess(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("name"), corelease.Request{Holder: "holder", Duration: time.Minute})
	c.Assert(err, jc.ErrorIsNil)

	leaseKey := corelease.Key{
		Namespace: defaultNamespace,
		ModelUUID: "model-uuid",
		Lease:     "name",
	}
	f, err := fix.Store.Trapdoor(leaseKey, "holder")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(f, gc.NotNil)
}
