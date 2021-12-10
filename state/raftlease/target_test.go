// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v2"
	txntesting "github.com/juju/txn/v2/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	coreraftlease "github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/raftlease"
)

const (
	collection = "testleaseholders"
)

type targetSuite struct {
	testing.IsolationSuite
	testing.MgoSuite
	db       *mgo.Database
	mongo    *Mongo
	errorLog loggo.Logger
}

var _ = gc.Suite(&targetSuite{})

func (s *targetSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *targetSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *targetSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.db = s.Session.DB("juju")
	s.mongo = NewMongo(s.db)
	s.errorLog = loggo.GetLogger("raftlease_test")
}

func (s *targetSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *targetSuite) newTarget() coreraftlease.NotifyTarget {
	return raftlease.NewNotifyTarget(s.mongo, collection, s.errorLog)
}

func (s *targetSuite) getRows(c *gc.C) []bson.M {
	var results []bson.M
	err := s.db.C(collection).Find(nil).Select(bson.M{
		"namespace":  true,
		"model-uuid": true,
		"lease":      true,
		"holder":     true,
	}).All(&results)
	c.Assert(err, jc.ErrorIsNil)
	return results
}

func (s *targetSuite) TestClaimedNoRecord(c *gc.C) {
	target := s.newTarget()
	err := target.Claimed(lease.Key{"ns", "model", "ankles"}, "tailpipe")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:ns#ankles#",
		"namespace":  "ns",
		"model-uuid": "model",
		"lease":      "ankles",
		"holder":     "tailpipe",
	}})
}

func (s *targetSuite) TestClaimedAlreadyHolder(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "voiid",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
}

func (s *targetSuite) TestClaimedDifferentHolder(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "shuffle",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
}

func (s *targetSuite) TestClaimedRecordsChangeBetweenAttempts(c *gc.C) {
	defer txntesting.SetBeforeHooks(c, s.mongo.runner, func() {
		err := s.db.C(collection).Insert(
			bson.M{
				"_id":        "model:leadership#twin#",
				"namespace":  "leadership",
				"model-uuid": "model",
				"lease":      "twin",
				"holder":     "kitamura",
			},
		)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	err := s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
}

func (s *targetSuite) TestClaimedError(c *gc.C) {
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("raftlease-target-tests", &logWriter), jc.ErrorIsNil)

	s.mongo.txnErr = errors.Errorf("oh no!")

	err := s.newTarget().Claimed(lease.Key{"ns", "model", "lease"}, "me")
	c.Assert(err, gc.ErrorMatches, `"model:ns#lease#" for "me" in db: oh no!`)
	c.Assert(s.getRows(c), gc.HasLen, 0)
	c.Assert(logWriter.Log(), jc.LogMatches, []string{
		`claiming lease "model:ns#lease#" for "me"`,
	})
}

func (s *targetSuite) TestExpired(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "kitamura",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin"},
		Holder: "kitamura",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpires(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin1#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin1",
			"holder":     "kitamura1",
		},
		bson.M{
			"_id":        "model:leadership#twin2#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin2",
			"holder":     "kitamura2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin1"},
		Holder: "kitamura1",
	}, {
		Key:    lease.Key{"leadership", "model", "twin2"},
		Holder: "kitamura2",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpiresWithDifferentHolder(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin1#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin1",
			"holder":     "kitamuraA",
		},
		bson.M{
			"_id":        "model:leadership#twin2#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin2",
			"holder":     "kitamura2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Attempt to expire this one, but it won't match. We should still expire
	// the second lease.
	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin1"},
		Holder: "kitamura1",
	}, {
		Key:    lease.Key{"leadership", "model", "twin2"},
		Holder: "kitamura2",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 1)

	// First write should win.
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin1#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin1",
		"holder":     "kitamuraA",
	}})
}

func (s *targetSuite) TestExpiresWithDuplicateEntries(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin1#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin1",
			"holder":     "kitamura1",
		},
		bson.M{
			"_id":        "model:leadership#twin2#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin2",
			"holder":     "kitamura2",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin1"},
		Holder: "kitamura1",
	}, {
		Key:    lease.Key{"leadership", "model", "twin2"},
		Holder: "kitamura2",
	}, {
		Key:    lease.Key{"leadership", "model", "twin1"},
		Holder: "kitamura1",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpiresWithMissingRecord(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin1#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin1",
			"holder":     "kitamura1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin1"},
		Holder: "kitamura1",
	}, {
		Key:    lease.Key{"leadership", "model", "twin2"},
		Holder: "kitamura2",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpiredNoRecord(c *gc.C) {
	err := s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin"},
		Holder: "kitamura",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpiredRemovedWhileRunning(c *gc.C) {
	coll := s.db.C(collection)
	err := coll.Insert(
		bson.M{
			"_id":        "model:leadership#twin#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "kitamura",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	defer txntesting.SetBeforeHooks(c, s.mongo.runner, func() {
		c.Assert(coll.Remove(nil), jc.ErrorIsNil)
	}).Check()

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin"},
		Holder: "kitamura",
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.getRows(c), gc.HasLen, 0)
}

func (s *targetSuite) TestExpiredError(c *gc.C) {
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("raftlease-target-tests", &logWriter), jc.ErrorIsNil)

	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:leadership#twin#",
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "kitamura",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.mongo.txnErr = errors.Errorf("oops!")

	err = s.newTarget().Expiries([]coreraftlease.Expired{{
		Key:    lease.Key{"leadership", "model", "twin"},
		Holder: "kitamura",
	}})
	c.Assert(err, gc.ErrorMatches, `\[model:leadership#twin#\] in db: oops!`)
	c.Assert(logWriter.Log(), jc.LogMatches, []string{
		`expiring leases \[model:leadership#twin#\]`,
	})
}

func (s *targetSuite) TestTrapdoorAttempt0(c *gc.C) {
	trapdoor := raftlease.MakeTrapdoorFunc(s.mongo, collection)(lease.Key{"ns", "model", "landfall"}, "roy")
	var result []txn.Op
	err := trapdoor(0, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []txn.Op{{
		C:      collection,
		Id:     "model:ns#landfall#",
		Assert: bson.M{"holder": "roy"},
	}})
	var bad int
	c.Assert(trapdoor(0, &bad), gc.ErrorMatches, `expected \*\[\]txn\.Op; \*int not valid`)
}

func (s *targetSuite) TestTrapdoorAttempt1NoHolderInDB(c *gc.C) {
	key := lease.Key{"ns", "model", "landfall"}
	trapdoor := raftlease.MakeTrapdoorFunc(s.mongo, collection)(key, "roy")
	var result []txn.Op
	err := trapdoor(1, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []txn.Op{{
		C:      collection,
		Id:     "model:ns#landfall#",
		Assert: bson.M{"holder": "roy"},
	}})
	// It also updated the database to make the holder roy.
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:ns#landfall#",
		"namespace":  "ns",
		"model-uuid": "model",
		"lease":      "landfall",
		"holder":     "roy",
	}})
}

func (s *targetSuite) TestTrapdoorAttempt1DifferentHolderInDB(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:ns#landfall#",
			"namespace":  "ns",
			"model-uuid": "model",
			"lease":      "landfall",
			"holder":     "george",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	key := lease.Key{"ns", "model", "landfall"}
	trapdoor := raftlease.MakeTrapdoorFunc(s.mongo, collection)(key, "roy")
	var result []txn.Op
	err = trapdoor(1, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []txn.Op{{
		C:      collection,
		Id:     "model:ns#landfall#",
		Assert: bson.M{"holder": "roy"},
	}})
	// It also updated the database to make the holder roy.
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:ns#landfall#",
		"namespace":  "ns",
		"model-uuid": "model",
		"lease":      "landfall",
		"holder":     "roy",
	}})
}

func (s *targetSuite) TestTrapdoorAttempt1ThisHolderInDB(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"_id":        "model:ns#landfall#",
			"namespace":  "ns",
			"model-uuid": "model",
			"lease":      "landfall",
			"holder":     "roy",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	trapdoor := raftlease.MakeTrapdoorFunc(s.mongo, collection)(lease.Key{"ns", "model", "landfall"}, "roy")
	var result []txn.Op
	err = trapdoor(0, &result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []txn.Op{{
		C:      collection,
		Id:     "model:ns#landfall#",
		Assert: bson.M{"holder": "roy"},
	}})
	// No change in the DB.
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:ns#landfall#",
		"namespace":  "ns",
		"model-uuid": "model",
		"lease":      "landfall",
		"holder":     "roy",
	}})
}

func (s *targetSuite) TestLeaseHolders(c *gc.C) {
	err := s.db.C(collection).Insert(
		bson.M{
			"namespace":  "singular",
			"model-uuid": "model",
			"lease":      "cogitans",
			"holder":     "planete",
		},
		bson.M{
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "cogitans",
			"holder":     "res",
		},
		bson.M{
			"namespace":  "leadership",
			"model-uuid": "model",
			"lease":      "twin",
			"holder":     "voiid",
		},
		bson.M{
			"namespace":  "leadership",
			"model-uuid": "model2",
			"lease":      "thorn",
			"holder":     "dornik",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	holders, err := raftlease.LeaseHolders(s.mongo, collection, "leadership", "model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(holders, gc.DeepEquals, map[string]string{
		"cogitans": "res",
		"twin":     "voiid",
	})
}

// Mongo exposes database operations. It uses a real database -- we can't mock
// mongo out, we need to check it really actually works -- but it's good to
// have the runner accessible for adversarial transaction tests.
type Mongo struct {
	database *mgo.Database
	runner   jujutxn.Runner
	txnErr   error
}

// NewMongo returns a *Mongo backed by the supplied database.
func NewMongo(database *mgo.Database) *Mongo {
	return &Mongo{
		database: database,
		runner: jujutxn.NewRunner(jujutxn.RunnerParams{
			Database: database,
		}),
	}
}

// GetCollection is part of the lease.Mongo interface.
func (m *Mongo) GetCollection(name string) (mongo.Collection, func()) {
	return mongo.CollectionFromName(m.database, name)
}

// RunTransaction is part of the lease.Mongo interface.
func (m *Mongo) RunTransaction(getTxn jujutxn.TransactionSource) error {
	if m.txnErr != nil {
		return m.txnErr
	}
	return m.runner.Run(getTxn)
}
