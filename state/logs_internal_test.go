// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type LogsInternalSuite struct {
	testing.MgoSuite
	testing.IsolationSuite
}

var _ = gc.Suite(&LogsInternalSuite{})

func (s *LogsInternalSuite) SetUpSuite(c *gc.C) {
	s.DebugMgo = true
	s.MgoSuite.SetUpSuite(c)
	s.IsolationSuite.SetUpSuite(c)
}

func (s *LogsInternalSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *LogsInternalSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.IsolationSuite.SetUpTest(c)
}

func (s *LogsInternalSuite) TearDownTest(c *gc.C) {
	err := s.Session.DB("logs").DropDatabase()
	c.Assert(err, jc.ErrorIsNil)
	s.IsolationSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *LogsInternalSuite) TestCollStatsForMissingDB(c *gc.C) {
	coll := s.Session.DB("logs").C("missing")
	_, err := collStats(coll)

	c.Assert(err.Error(), gc.Equals, "Collection [logs.missing] not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *LogsInternalSuite) createLogsDB(c *gc.C) *mgo.Collection {
	// We create the db by writing something into a collection.
	coll := s.Session.DB("logs").C("new")
	err := coll.Insert(bson.M{"_id": "new"})
	c.Assert(err, jc.ErrorIsNil)
	return coll
}

func (s *LogsInternalSuite) createCapped(c *gc.C, name string, size int) *mgo.Collection {
	// We create the db by writing something into a collection.
	coll := s.Session.DB("logs").C(name)
	err := coll.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: size,
	})
	c.Assert(err, jc.ErrorIsNil)
	return coll
}

func (s *LogsInternalSuite) TestCollStatsForMissingCollection(c *gc.C) {
	// Create a collection in the logs database to make sure the DB exists.
	s.createLogsDB(c)

	coll := s.Session.DB("logs").C("missing")
	_, err := collStats(coll)

	c.Assert(err.Error(), gc.Equals, "Collection [logs.missing] not found")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *LogsInternalSuite) dbLogger(coll *mgo.Collection) *DbLogger {
	return &DbLogger{
		logsColl:  coll,
		modelUUID: "fake-uuid",
	}
}
