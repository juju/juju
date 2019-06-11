// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/kr/pretty"
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

func (s *LogsInternalSuite) TestCollStatsForLogsDB(c *gc.C) {
	coll := s.Session.DB("logs").C("missing")
	_, err := collStats(coll)

	c.Assert(err.Error(), gc.Equals, "Database [logs] not found.")
}

func (s *LogsInternalSuite) createLogsDB(c *gc.C) {
	// We create the db by writing something into a collection.
	coll := s.Session.DB("logs").C("new")
	err := coll.Insert(bson.M{"_id": "new"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LogsInternalSuite) createCapped(c *gc.C, name string, size int) {
	// We create the db by writing something into a collection.
	coll := s.Session.DB("logs").C(name)
	err := coll.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: size,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LogsInternalSuite) TestCollStatsForMissingCollection(c *gc.C) {
	s.createLogsDB(c)

	coll := s.Session.DB("logs").C("missing")
	_, err := collStats(coll)

	c.Assert(err.Error(), gc.Equals, "Collection [logs.missing] not found.")
}

func (s *LogsInternalSuite) TestCollStatsForNewCollection(c *gc.C) {
	s.createLogsDB(c)

	coll := s.Session.DB("logs").C("new")
	result, err := collStats(coll)

	c.Logf(pretty.Sprint(result))
	c.Logf(pretty.Sprint(err))

	c.Fail()
}

func (s *LogsInternalSuite) TestCollStatsForCappedCollection(c *gc.C) {
	// mgo MaxBytes is in bytes, whereas we query in MB
	s.createCapped(c, "capped", 20*1024*1024)

	coll := s.Session.DB("logs").C("capped")
	result, err := collStats(coll)

	c.Logf(pretty.Sprint(result))
	c.Logf(pretty.Sprint(err))

	c.Fail()
}
