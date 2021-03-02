// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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

func (s *LogsInternalSuite) TestCappedInfoForNormalCollection(c *gc.C) {
	coll := s.createLogsDB(c)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsFalse)
	c.Assert(maxSize, gc.Equals, 0)
}

func (s *LogsInternalSuite) TestCappedInfoForCappedCollection(c *gc.C) {
	// mgo MaxBytes is in bytes, whereas we query in MB
	coll := s.createCapped(c, "capped", 20*1024*1024)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Assert(maxSize, gc.Equals, 20)
}

func (s *LogsInternalSuite) dbLogger(coll *mgo.Collection) *DbLogger {
	return &DbLogger{
		logsColl:  coll,
		modelUUID: "fake-uuid",
	}
}

func writeSomeLogs(c *gc.C, logger *DbLogger, count int) {
	toWrite := make([]LogRecord, 0, count)
	when := time.Now()
	c.Logf("writing %d logs to the db\n", count)
	for i := 0; i < count; i++ {
		toWrite = append(toWrite, LogRecord{
			Time:     when,
			Entity:   "some-entity",
			Level:    loggo.DEBUG,
			Module:   "test",
			Location: "test_file.go:1234",
			Message:  fmt.Sprintf("test line %d", i),
		})
	}
	err := logger.Log(toWrite)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LogsInternalSuite) TestLogsCollectionConversion(c *gc.C) {
	coll := s.createLogsDB(c)
	err := convertToCapped(coll, 5)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Assert(maxSize, gc.Equals, 5)
}

func (s *LogsInternalSuite) TestLogsCollectionConversionTwice(c *gc.C) {
	coll := s.createLogsDB(c)
	err := convertToCapped(coll, 5)
	c.Assert(err, jc.ErrorIsNil)

	err = convertToCapped(coll, 10)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Assert(maxSize, gc.Equals, 10)
}

func (s *LogsInternalSuite) TestLogsCollectionConversionSmallerSize(c *gc.C) {
	// Create a log db that has two meg of data, then convert it
	// to a capped collection with a one meg limit.
	coll := s.createLogsDB(c)
	dbLogger := s.dbLogger(coll)
	size := 0
	var err error
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		size, err = getCollectionMB(coll)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Assert(maxSize, gc.Equals, 2)

	size, err = getCollectionMB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(size, jc.LessThan, 3)

	// Check that we still have some documents in there.
	docs, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.GreaterThan, 5000)
}

func (s *LogsInternalSuite) TestLogsCollectionConversionTwiceSmallerSize(c *gc.C) {
	// Create a log db that has two meg of data, then convert it
	// to a capped collection with a one meg limit.
	coll := s.createLogsDB(c)
	dbLogger := s.dbLogger(coll)
	size := 0
	var err error
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		size, err = getCollectionMB(coll)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = convertToCapped(coll, 10)
	c.Assert(err, jc.ErrorIsNil)

	// Now resize again to 2.
	err = convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Check(maxSize, gc.Equals, 2)

	size, err = getCollectionMB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(size, jc.LessThan, 3)

	// Check that we still have some documents in there.
	docs, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.GreaterThan, 5000)
}

func (s *LogsInternalSuite) TestLogsCollectionConversionTwiceBiggerSize(c *gc.C) {
	// Create a log db that has two meg of data, then convert it
	// to a capped collection with a one meg limit.
	coll := s.createLogsDB(c)
	dbLogger := s.dbLogger(coll)
	size := 0
	var err error
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		size, err = getCollectionMB(coll)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = convertToCapped(coll, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Now resize again to 2.
	err = convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Check(maxSize, gc.Equals, 2)

	size, err = getCollectionMB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(size, jc.LessThan, 3)

	// Check that we still have some documents in there.
	docs, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.GreaterThan, 5000)
}
