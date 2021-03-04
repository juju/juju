// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"bytes"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lease"
	coreraftlease "github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/raftlease"
)

const (
	collection = "testleaseholders"
	logPrefix  = `\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{1,6} `
)

type targetSuite struct {
	testing.IsolationSuite
	testing.MgoSuite
	db       *mgo.Database
	mongo    *Mongo
	errorLog loggo.Logger
	leaseLog *bytes.Buffer
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
	s.leaseLog = bytes.NewBuffer(nil)
}

func (s *targetSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *targetSuite) newTarget() coreraftlease.NotifyTarget {
	return raftlease.NewNotifyTarget(s.mongo, collection, s.leaseLog, s.errorLog)
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
	target.Claimed(lease.Key{"ns", "model", "ankles"}, "tailpipe")
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:ns#ankles#",
		"namespace":  "ns",
		"model-uuid": "model",
		"lease":      "ankles",
		"holder":     "tailpipe",
	}})
	s.assertLogMatches(c, `claimed "model:ns#ankles#" for "tailpipe"`)
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
	s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
	s.assertLogMatches(c, `claimed "model:leadership#twin#" for "voiid"`)
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
	s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
	s.assertLogMatches(c, `claimed "model:leadership#twin#" for "voiid"`)
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
	s.newTarget().Claimed(
		lease.Key{"leadership", "model", "twin"},
		"voiid",
	)
	c.Assert(s.getRows(c), gc.DeepEquals, []bson.M{{
		"_id":        "model:leadership#twin#",
		"namespace":  "leadership",
		"model-uuid": "model",
		"lease":      "twin",
		"holder":     "voiid",
	}})
	s.assertLogMatches(c, `claimed "model:leadership#twin#" for "voiid"`)
}

func (s *targetSuite) TestClaimedError(c *gc.C) {
	var logWriter loggo.TestWriter
	c.Assert(loggo.RegisterWriter("raftlease-target-tests", &logWriter), jc.ErrorIsNil)
	s.mongo.txnErr = errors.Errorf("oh no!")
	s.newTarget().Claimed(lease.Key{"ns", "model", "lease"}, "me")
	c.Assert(s.getRows(c), gc.HasLen, 0)
	c.Assert(logWriter.Log(), jc.LogMatches, []string{
		`couldn't claim lease "model:ns#lease#" for "me" in db: oh no!`,
	})
	s.assertLogMatches(c,
		`claimed "model:ns#lease#" for "me"`,
		`couldn't claim lease "model:ns#lease#" for "me" in db: oh no!`,
	)
}

func (s *targetSuite) assertLogMatches(c *gc.C, expectLines ...string) {
	lines := strings.Split(string(s.leaseLog.Bytes()), "\n")
	c.Assert(lines, gc.HasLen, len(expectLines)+1)
	for i, expected := range expectLines {
		c.Assert(lines[i], gc.Matches, logPrefix+expected, gc.Commentf("line %d", i))
	}
	c.Assert(lines[len(expectLines)], gc.Equals, "")
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
	s.newTarget().Expired(lease.Key{"leadership", "model", "twin"})
	c.Assert(s.getRows(c), gc.HasLen, 0)
	s.assertLogMatches(c, `expired "model:leadership#twin#"`)
}

func (s *targetSuite) TestExpiredNoRecord(c *gc.C) {
	s.newTarget().Expired(lease.Key{"leadership", "model", "twin"})
	c.Assert(s.getRows(c), gc.HasLen, 0)
	s.assertLogMatches(c, `expired "model:leadership#twin#"`)
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
	s.newTarget().Expired(lease.Key{"leadership", "model", "twin"})
	c.Assert(s.getRows(c), gc.HasLen, 0)
	s.assertLogMatches(c, `expired "model:leadership#twin#"`)
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
	s.newTarget().Expired(lease.Key{"leadership", "model", "twin"})
	c.Assert(logWriter.Log(), jc.LogMatches, []string{
		`couldn't expire lease "model:leadership#twin#" in db: oops!`,
	})
	s.assertLogMatches(c,
		`expired "model:leadership#twin#"`,
		`couldn't expire lease "model:leadership#twin#" in db: oops!`,
	)
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
