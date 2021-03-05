// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/mgo/v2/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

func (s *upgradesSuite) TestMigrateLeasesToGlobalTime(c *gc.C) {
	leases, closer := s.state.db().GetRawCollection("leases")
	defer closer()

	// Use the non-controller model to ensure we can run the function
	// across multiple models.
	otherState := s.makeModel(c, "crack-up", coretesting.Attrs{})
	defer otherState.Close()

	uuid := otherState.ModelUUID()

	err := leases.Insert(bson.M{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, bson.M{
		"_id":        uuid + ":clock#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "clock",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	})
	c.Assert(err, jc.ErrorIsNil)

	// - garbage doc is left alone has it has no "type" field
	// - clock doc is removed, but no replacement required
	// - lease doc is removed and replaced
	expectedLeases := []bson.M{{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, {
		"_id":        uuid + ":some-namespace#some-name#",
		"model-uuid": uuid,
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "ghost",
	}}
	s.assertUpgradedData(c, MigrateLeasesToGlobalTime,
		upgradedData(leases, expectedLeases),
	)
}

func (s *upgradesSuite) TestMigrateLeasesToGlobalTimeWithNewTarget(c *gc.C) {
	// It is possible that API servers will try to coordinate the singular lease before we can get to the upgrade steps.
	// While upgrading leases, if we encounter any leases that already exist in the new GlobalTime format, they should
	// be considered authoritative, and the old lease should just be deleted.
	leases, closer := s.state.db().GetRawCollection("leases")
	defer closer()

	// Use the non-controller model to ensure we can run the function
	// across multiple models.
	otherState := s.makeModel(c, "crack-up", coretesting.Attrs{})
	defer otherState.Close()

	uuid := otherState.ModelUUID()

	err := leases.Insert(bson.M{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, bson.M{
		"_id":        uuid + ":clock#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "clock",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace2#some-name2#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	}, bson.M{
		// some-namespace2 has already been created in the new format
		"_id":        uuid + ":some-namespace2#some-name2#",
		"model-uuid": uuid,
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "foot",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "gobble",
	})
	c.Assert(err, jc.ErrorIsNil)

	// - garbage doc is left alone has it has no "type" field
	// - clock doc is removed, but no replacement required
	// - lease doc is removed and replaced
	// - second old lease doc is removed, and the new lease doc is not overwritten
	expectedLeases := []bson.M{{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, {
		"_id":        uuid + ":some-namespace#some-name#",
		"model-uuid": uuid,
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "ghost",
	}, {
		"_id":        uuid + ":some-namespace2#some-name2#",
		"model-uuid": uuid,
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "foot",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "gobble",
	}}
	s.assertUpgradedData(c, MigrateLeasesToGlobalTime,
		upgradedData(leases, expectedLeases),
	)
}

func (s *upgradesSuite) TestLegacyLeases(c *gc.C) {
	clockColl, clockCloser := s.state.db().GetCollection(globalClockC)
	defer clockCloser()
	c.Assert(clockColl.Writeable().Insert(bson.M{
		"_id":  "g",
		"time": int64(5000000000),
	}), jc.ErrorIsNil)

	coll, closer := s.state.db().GetRawCollection("leases")
	defer closer()
	err := coll.Insert(bson.M{
		"namespace":  "buke",
		"model-uuid": "m1",
		"name":       "seam-esteem",
		"holder":     "gase",
		"start":      int64(4000000000),
		"duration":   5 * time.Second,
	}, bson.M{
		"namespace":  "reyne",
		"model-uuid": "m2",
		"name":       "scorned",
		"holder":     "jordan",
		"start":      int64(4500000000),
		"duration":   10 * time.Second,
	})
	c.Assert(err, jc.ErrorIsNil)

	now, err := time.Parse(time.RFC3339Nano, "2018-09-13T10:51:00.300000000Z")
	c.Assert(err, jc.ErrorIsNil)
	result, err := LegacyLeases(s.pool, now)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, map[lease.Key]lease.Info{
		{"buke", "m1", "seam-esteem"}: {
			Holder:   "gase",
			Expiry:   now.Add(4 * time.Second),
			Trapdoor: nil,
		},
		{"reyne", "m2", "scorned"}: {
			Holder:   "jordan",
			Expiry:   now.Add(9500 * time.Millisecond),
			Trapdoor: nil,
		},
	})
}

func (s *upgradesSuite) TestDropLeasesCollection(c *gc.C) {
	db := s.state.session.DB("juju")
	col := db.C("leases")
	err := col.Insert(bson.M{"test": "foo"})
	c.Assert(err, jc.ErrorIsNil)
	// Sanity check.
	names, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(names...).Contains("leases"), jc.IsTrue)

	err = DropLeasesCollection(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	names, err = db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(names...).Contains("leases"), jc.IsFalse)

	err = DropLeasesCollection(s.pool)
	c.Assert(err, jc.ErrorIsNil)
}
