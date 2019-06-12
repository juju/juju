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
	coll, closer := db.GetCollection(defaultCollection)
	defer closer()
	_, err := statelease.LookupLease(coll, defaultNamespace, "bar")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *StoreSuite) TestLookupLease(c *gc.C) {
	fix := s.EasyFixture(c)
	err := fix.Store.ClaimLease(key("name"), corelease.Request{Holder: "holder", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	db := NewMongo(s.db)
	coll, closer := db.GetCollection(defaultCollection)
	defer closer()
	doc, err := statelease.LookupLease(coll, defaultNamespace, "name")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(doc.Name, gc.Equals, "name")
	c.Check(doc.Holder, gc.Equals, "holder")
	c.Check(doc.Namespace, gc.Equals, defaultNamespace)
}

func (s *StoreSuite) TestLeasesNoFilter(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("duck"), corelease.Request{Holder: "donald", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Store.ClaimLease(key("mouse"), corelease.Request{Holder: "mickey", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)

	leases := fix.Store.Leases()
	c.Check(leases, gc.HasLen, 2)
	c.Check(leases[key("duck")].Holder, gc.Equals, "donald")
	c.Check(leases[key("mouse")].Holder, gc.Equals, "mickey")
}

func (s *StoreSuite) TestLeasesFilter(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("duck"), corelease.Request{Holder: "donald", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Store.ClaimLease(key("mouse"), corelease.Request{Holder: "mickey", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)

	// One key with an unheld lease, one with an invalid namespace, one with an invalid model.
	leases := fix.Store.Leases(
		key("dog"),
		corelease.Key{Lease: "duck", ModelUUID: "model-uuid", Namespace: "nope"},
		corelease.Key{Lease: "mouse", ModelUUID: "nope", Namespace: defaultNamespace},
	)

	c.Check(len(leases), gc.Equals, 0)
}

func (s *StoreSuite) TestLeaseGroup(c *gc.C) {
	fix := s.EasyFixture(c)

	err := fix.Store.ClaimLease(key("duck"), corelease.Request{Holder: "donald", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = fix.Store.ClaimLease(key("mouse"), corelease.Request{Holder: "mickey", Duration: time.Minute}, nil)
	c.Assert(err, jc.ErrorIsNil)

	leases := fix.Store.LeaseGroup(fix.Config.Namespace, fix.Config.ModelUUID)
	c.Check(leases, gc.HasLen, 2)
	c.Check(leases[key("duck")].Holder, gc.Equals, "donald")
	c.Check(leases[key("mouse")].Holder, gc.Equals, "mickey")

	leases = fix.Store.LeaseGroup("otherns", fix.Config.ModelUUID)
	c.Assert(leases, gc.HasLen, 0)

	leases = fix.Store.LeaseGroup(fix.Config.Namespace, "othermodel")
	c.Assert(leases, gc.HasLen, 0)
}
