// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelogger "github.com/juju/juju/core/logger"
)

type LogsInternalSuite struct {
	mgotesting.MgoSuite
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	toWrite := make([]corelogger.LogRecord, 0, count)
	when := time.Now()
	c.Logf("writing %d logs to the db\n", count)
	for i := 0; i < count; i++ {
		toWrite = append(toWrite, corelogger.LogRecord{
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
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		sizeKB, err := getCollectionKB(coll)
		c.Assert(err, jc.ErrorIsNil)
		size = sizeKB / humanize.KiByte
	}

	err := convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Assert(maxSize, gc.Equals, 2)

	sizeKB, err := getCollectionKB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(sizeKB/humanize.KiByte, jc.LessThan, 3)

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
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		sizeKB, err := getCollectionKB(coll)
		c.Assert(err, jc.ErrorIsNil)
		size = sizeKB / humanize.KiByte
	}

	err := convertToCapped(coll, 10)
	c.Assert(err, jc.ErrorIsNil)

	// Now resize again to 2.
	err = convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Check(maxSize, gc.Equals, 2)

	sizeKB, err := getCollectionKB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(sizeKB/humanize.KiByte, jc.LessThan, 3)

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
	for size < 4 {
		writeSomeLogs(c, dbLogger, 5000)
		sizeKB, err := getCollectionKB(coll)
		c.Assert(err, jc.ErrorIsNil)
		size = sizeKB / humanize.KiByte
	}

	err := convertToCapped(coll, 1)
	c.Assert(err, jc.ErrorIsNil)

	// Now resize again to 2.
	err = convertToCapped(coll, 2)
	c.Assert(err, jc.ErrorIsNil)

	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Check(maxSize, gc.Equals, 2)

	sizeKB, err := getCollectionKB(coll)
	c.Assert(err, jc.ErrorIsNil)
	// We don't have a LessThan or equal to, so using 3 to mean 1 or 2.
	c.Assert(sizeKB/humanize.KiByte, jc.LessThan, 3)

	// Check that we still have some documents in there.
	docs, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.GreaterThan, 5000)
}

type DBLogInitInternalSuite struct {
	mgotesting.MgoSuite
	testing.IsolationSuite
}

var _ = gc.Suite(&DBLogInitInternalSuite{})

func (s *DBLogInitInternalSuite) SetUpSuite(c *gc.C) {
	s.DebugMgo = true
	s.MgoSuite.SetUpSuite(c)
	s.IsolationSuite.SetUpSuite(c)
}

func (s *DBLogInitInternalSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *DBLogInitInternalSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.IsolationSuite.SetUpTest(c)
}

func (s *DBLogInitInternalSuite) TearDownTest(c *gc.C) {
	err := s.Session.DB("logs").DropDatabase()
	c.Assert(err, jc.ErrorIsNil)
	s.IsolationSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

func (s *DBLogInitInternalSuite) TestInitDBLogsForModel(c *gc.C) {
	err := InitDbLogsForModel(s.Session, "foo-bar", 10)
	c.Assert(err, jc.ErrorIsNil)

	err = InitDbLogsForModel(s.Session, "foo-bar", 10)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DBLogInitInternalSuite) TestInitDBLogsForModelConcurrently(c *gc.C) {
	results := make(chan error)
	defer close(results)

	for i := 0; i < 2; i++ {
		go func() {
			results <- InitDbLogsForModel(s.Session, "foo-bar", 10)
		}()
	}
	var amount int
LOOP:
	for {
		select {
		case err := <-results:
			c.Assert(err, jc.ErrorIsNil)
			amount++
			if amount == 2 {
				break LOOP
			}
		case <-time.After(testing.LongWait):
			c.Fatal("error timedout waiting for a result")
			return
		}
	}

	c.Assert(amount, gc.Equals, 2)

	coll := s.Session.DB("logs").C(logCollectionName("foo-bar"))
	capped, maxSize, err := getCollectionCappedInfo(coll)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(capped, jc.IsTrue)
	c.Check(maxSize, gc.Equals, 10)
}
