// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock_test

import (
	// Only used for time types.
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state/globalclock"
)

type ReaderSuite struct {
	testing.MgoSuite
	config globalclock.ReaderConfig
}

var _ = gc.Suite(&ReaderSuite{})

const pollInterval = 30 * time.Second

func (s *ReaderSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.Session.DB(database).DropDatabase()
	s.config = globalclock.ReaderConfig{
		Config: globalclock.Config{
			Database:   database,
			Collection: collection,
			Session:    s.Session,
		},
	}
}

func (s *ReaderSuite) TestNewReaderValidatesConfigDatabase(c *gc.C) {
	s.config.Database = ""
	_, err := globalclock.NewReader(s.config)
	c.Assert(err, gc.ErrorMatches, "missing database")
}

func (s *ReaderSuite) TestNewReaderValidatesConfigCollection(c *gc.C) {
	s.config.Collection = ""
	_, err := globalclock.NewReader(s.config)
	c.Assert(err, gc.ErrorMatches, "missing collection")
}

func (s *ReaderSuite) TestNewReaderValidatesConfigSession(c *gc.C) {
	s.config.Session = nil
	_, err := globalclock.NewReader(s.config)
	c.Assert(err, gc.ErrorMatches, "missing mongo session")
}

func (s *ReaderSuite) TestNewReaderInitialValue(c *gc.C) {
	s.writeTime(c, globalEpoch.Add(time.Second))

	r := s.newReader(c)
	c.Assert(r, gc.NotNil)

	t, err := r.Now()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, gc.Equals, globalEpoch.Add(time.Second))
}

func (s *ReaderSuite) TestNewReaderInitialValueMissing(c *gc.C) {
	r := s.newReader(c)
	c.Assert(r, gc.NotNil)

	t, err := r.Now()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(t, gc.Equals, globalEpoch)
}

func (s *ReaderSuite) newReader(c *gc.C) *globalclock.Reader {
	r, err := globalclock.NewReader(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *ReaderSuite) writeTime(c *gc.C, t time.Time) {
	coll := s.Session.DB(database).C(collection)
	_, err := coll.UpsertId("g", bson.D{{
		"$set", bson.D{{"time", t.UnixNano()}},
	}})
	c.Assert(err, jc.ErrorIsNil)
}
