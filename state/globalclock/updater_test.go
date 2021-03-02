// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock_test

import (
	// Only used for time types.
	"time"

	mgo "github.com/juju/mgo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreglobalclock "github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/globalclock"
)

type UpdaterSuite struct {
	testing.MgoSuite
	config globalclock.UpdaterConfig
}

var _ = gc.Suite(&UpdaterSuite{})

const (
	database   = "testing"
	collection = "globalclock"
)

var globalEpoch = time.Unix(0, 0)

func (s *UpdaterSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.Session.DB(database).DropDatabase()
	s.config = globalclock.UpdaterConfig{
		globalclock.Config{
			Collection: collection,
			Mongo:      mongoWrapper{s.Session},
		},
	}
}

func (s *UpdaterSuite) TestNewUpdaterValidatesConfigCollection(c *gc.C) {
	s.config.Collection = ""
	_, err := globalclock.NewUpdater(s.config)
	c.Assert(err, gc.ErrorMatches, "missing collection")
}

func (s *UpdaterSuite) TestNewUpdaterValidatesConfigMongo(c *gc.C) {
	s.config.Mongo = nil
	_, err := globalclock.NewUpdater(s.config)
	c.Assert(err, gc.ErrorMatches, "missing mongo client")
}

func (s *UpdaterSuite) TestNewUpdaterCreatesEpochDocument(c *gc.C) {
	u := s.newUpdater(c)
	c.Assert(u, gc.NotNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch)
}

func (s *UpdaterSuite) TestAdvance(c *gc.C) {
	u := s.newUpdater(c)
	for i := 0; i < 2; i++ {
		err := u.Advance(time.Second+time.Nanosecond, nil)
		c.Assert(err, jc.ErrorIsNil)
		expectedTime := globalEpoch.Add(time.Duration(i+1) * (time.Second + time.Nanosecond))
		c.Assert(s.readTime(c), gc.Equals, expectedTime)
	}
}

func (s *UpdaterSuite) TestNewUpdaterPreservesTime(c *gc.C) {
	u0 := s.newUpdater(c)
	err := u0.Advance(time.Second, nil)
	c.Assert(err, jc.ErrorIsNil)

	u1 := s.newUpdater(c)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second))

	err = u1.Advance(time.Second, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(2*time.Second))
}

func (s *UpdaterSuite) TestUpdaterConcurrentAdvance(c *gc.C) {
	u0 := s.newUpdater(c)
	u1 := s.newUpdater(c)

	err := u0.Advance(time.Second, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second))

	err = u1.Advance(time.Second, nil)
	c.Assert(err, gc.Equals, coreglobalclock.ErrOutOfSyncUpdate)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second)) // no change

	// u1's view of the clock should have been updated when
	// ErrOutOfSyncUpdate was returned, so Advance should
	// now succeed.
	err = u1.Advance(time.Second, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(2*time.Second))
}

func (s *UpdaterSuite) newUpdater(c *gc.C) *globalclock.Updater {
	u, err := globalclock.NewUpdater(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return u
}

func (s *UpdaterSuite) readTime(c *gc.C) time.Time {
	type doc struct {
		DocID string `bson:"_id"`
		Time  int64  `bson:"time"`
	}

	var docs []doc
	coll := s.Session.DB(database).C(collection)
	err := coll.Find(nil).Sort("_id").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0].DocID, gc.Equals, "g")
	return time.Unix(0, docs[0].Time)
}

type mongoWrapper struct {
	sess *mgo.Session
}

func (w mongoWrapper) GetCollection(collName string) (mongo.Collection, func()) {
	return mongo.CollectionFromName(w.sess.DB(database), collName)
}
