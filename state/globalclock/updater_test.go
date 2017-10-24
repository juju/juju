// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock_test

import (
	// Only used for time types.
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreglobalclock "github.com/juju/juju/core/globalclock"
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
			Database:   database,
			Collection: collection,
			Session:    s.Session,
		},
	}
}

func (s *UpdaterSuite) TestNewUpdaterValidatesConfigDatabase(c *gc.C) {
	s.config.Database = ""
	_, err := globalclock.NewUpdater(s.config)
	c.Assert(err, gc.ErrorMatches, "missing database")
}

func (s *UpdaterSuite) TestNewUpdaterValidatesConfigCollection(c *gc.C) {
	s.config.Collection = ""
	_, err := globalclock.NewUpdater(s.config)
	c.Assert(err, gc.ErrorMatches, "missing collection")
}

func (s *UpdaterSuite) TestNewUpdaterValidatesConfigSession(c *gc.C) {
	s.config.Session = nil
	_, err := globalclock.NewUpdater(s.config)
	c.Assert(err, gc.ErrorMatches, "missing mongo session")
}

func (s *UpdaterSuite) TestNewUpdaterCreatesEpochDocument(c *gc.C) {
	u := s.newUpdater(c)
	c.Assert(u, gc.NotNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch)
}

func (s *UpdaterSuite) TestAddTime(c *gc.C) {
	u := s.newUpdater(c)
	for i := 0; i < 2; i++ {
		err := u.AddTime(time.Second)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Duration(i+1)*time.Second))
	}
}

func (s *UpdaterSuite) TestNewUpdaterPreservesTime(c *gc.C) {
	u0 := s.newUpdater(c)
	err := u0.AddTime(time.Second)
	c.Assert(err, jc.ErrorIsNil)

	u1 := s.newUpdater(c)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second))

	err = u1.AddTime(time.Second)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(2*time.Second))
}

func (s *UpdaterSuite) TestUpdaterConcurrentAddTime(c *gc.C) {
	u0 := s.newUpdater(c)
	u1 := s.newUpdater(c)

	err := u0.AddTime(time.Second)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second))

	err = u1.AddTime(time.Second)
	c.Assert(err, gc.Equals, coreglobalclock.ErrConcurrentUpdate)
	c.Assert(s.readTime(c), gc.Equals, globalEpoch.Add(time.Second)) // no change

	// u1's view of the clock should have been updated when
	// ErrConcurrentUpdate was returned, so AddTime should
	// now succeed.
	err = u1.AddTime(time.Second)
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
