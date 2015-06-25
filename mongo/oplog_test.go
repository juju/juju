// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/peergrouper"
)

type oplogSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&oplogSuite{})

func (s *oplogSuite) TestWithRealOplog(c *gc.C) {
	_, session := s.startMongoWithReplicaset(c)

	// Watch for oplog entries for the "bar" collection in the "foo"
	// DB.
	oplog := mongo.GetOplog(session)
	tailer := mongo.NewOplogTailer(
		oplog,
		bson.D{{"ns", "foo.bar"}},
		time.Now().Add(-time.Minute),
	)
	defer tailer.Stop()

	assertOplog := func(expectedOp string, expectedObj, expectedUpdate bson.D) {
		doc := s.getNextOplog(c, tailer)
		c.Assert(doc.Operation, gc.Equals, expectedOp)

		var actualObj bson.D
		err := doc.UnmarshalObject(&actualObj)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actualObj, jc.DeepEquals, expectedObj)

		var actualUpdate bson.D
		err = doc.UnmarshalUpdate(&actualUpdate)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actualUpdate, jc.DeepEquals, expectedUpdate)
	}

	// Insert into foo.bar and see that the oplog entry is reported.
	db := session.DB("foo")
	coll := db.C("bar")
	s.insertDoc(c, session, coll, bson.M{"_id": "thing"})
	assertOplog("i", bson.D{{"_id", "thing"}}, nil)

	// Update foo.bar and see the update reported.
	err := coll.UpdateId("thing", bson.M{"$set": bson.M{"blah": 42}})
	c.Assert(err, jc.ErrorIsNil)
	assertOplog("u", bson.D{{"$set", bson.D{{"blah", 42}}}}, bson.D{{"_id", "thing"}})

	// Insert into another collection (shouldn't be reported due to filter).
	s.insertDoc(c, session, db.C("elsewhere"), bson.M{"_id": "boo"})
	s.assertNoOplog(c, tailer)
}

func (s *oplogSuite) TestHonoursInitialTs(c *gc.C) {
	_, session := s.startMongo(c)

	t := time.Now()

	oplog := s.makeFakeOplog(c, session)
	for offset := -1; offset <= 1; offset++ {
		tDoc := t.Add(time.Duration(offset) * time.Second)
		s.insertDoc(c, session, oplog,
			&mongo.OplogDoc{Timestamp: mongo.NewMongoTimestamp(tDoc)},
		)
	}

	tailer := mongo.NewOplogTailer(oplog, nil, t)
	defer tailer.Stop()

	for offset := 0; offset <= 1; offset++ {
		doc := s.getNextOplog(c, tailer)
		tExpected := t.Add(time.Duration(offset) * time.Second)
		c.Assert(doc.Timestamp, gc.Equals, mongo.NewMongoTimestamp(tExpected))
	}
}

func (s *oplogSuite) TestStops(c *gc.C) {
	_, session := s.startMongo(c)

	oplog := s.makeFakeOplog(c, session)
	tailer := mongo.NewOplogTailer(oplog, nil, time.Time{})
	defer tailer.Stop()

	s.insertDoc(c, session, oplog, &mongo.OplogDoc{Timestamp: 1})
	s.getNextOplog(c, tailer)

	err := tailer.Stop()
	c.Assert(err, jc.ErrorIsNil)

	s.assertStopped(c, tailer)
	c.Assert(tailer.Err(), jc.ErrorIsNil)
}

func (s *oplogSuite) TestRestartsOnError(c *gc.C) {
	_, session := s.startMongo(c)

	oplog := s.makeFakeOplog(c, session)
	tailer := mongo.NewOplogTailer(oplog, nil, time.Time{})
	defer tailer.Stop()

	// First, ensure that the tailer is seeing oplog inserts.
	s.insertDoc(c, session, oplog, &mongo.OplogDoc{
		Timestamp:   1,
		OperationId: 99,
	})
	doc := s.getNextOplog(c, tailer)
	c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(1))

	s.emptyCapped(c, oplog)

	// Ensure that the tailer still works.
	s.insertDoc(c, session, oplog, &mongo.OplogDoc{
		Timestamp:   2,
		OperationId: 42,
	})
	doc = s.getNextOplog(c, tailer)
	c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(2))
}

func (s *oplogSuite) TestNoRepeatsAfterIterRestart(c *gc.C) {
	_, session := s.startMongo(c)

	oplog := s.makeFakeOplog(c, session)
	tailer := mongo.NewOplogTailer(oplog, nil, time.Time{})
	defer tailer.Stop()

	// Insert a bunch of oplog entries with the same timestamp (but
	// with different ids) and see them reported.
	for id := int64(10); id < 15; id++ {
		s.insertDoc(c, session, oplog, &mongo.OplogDoc{
			Timestamp:   1,
			OperationId: id,
		})

		doc := s.getNextOplog(c, tailer)
		c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(1))
		c.Assert(doc.OperationId, gc.Equals, id)
	}

	// Force the OplogTailer's iterator to be recreated.
	s.emptyCapped(c, oplog)

	// Reinsert the oplog entries that were already there before and a
	// few more.
	for id := int64(10); id < 20; id++ {
		s.insertDoc(c, session, oplog, &mongo.OplogDoc{
			Timestamp:   1,
			OperationId: id,
		})
	}

	// Insert an entry for a later timestamp.
	s.insertDoc(c, session, oplog, &mongo.OplogDoc{
		Timestamp:   2,
		OperationId: 42,
	})

	// Ensure that only previously unreported entries are now reported.
	for id := int64(15); id < 20; id++ {
		doc := s.getNextOplog(c, tailer)
		c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(1))
		c.Assert(doc.OperationId, gc.Equals, id)
	}

	doc := s.getNextOplog(c, tailer)
	c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(2))
	c.Assert(doc.OperationId, gc.Equals, int64(42))
}

func (s *oplogSuite) TestDiesOnFatalError(c *gc.C) {
	_, session := s.startMongo(c)
	oplog := s.makeFakeOplog(c, session)
	s.insertDoc(c, session, oplog, &mongo.OplogDoc{Timestamp: 1})

	tailer := mongo.NewOplogTailer(oplog, nil, time.Time{})
	defer tailer.Stop()

	doc := s.getNextOplog(c, tailer)
	c.Assert(doc.Timestamp, gc.Equals, bson.MongoTimestamp(1))

	// Induce a fatal error by removing the oplog collection.
	err := oplog.DropCollection()
	c.Assert(err, jc.ErrorIsNil)

	s.assertStopped(c, tailer)
	// The actual error varies by MongoDB version so just check that
	// there is one.
	c.Assert(tailer.Err(), gc.Not(jc.ErrorIsNil))
}

func (s *oplogSuite) TestNewMongoTimestamp(c *gc.C) {
	t := time.Date(2015, 6, 24, 12, 47, 0, 0, time.FixedZone("somewhere", 5*3600))

	expected := bson.MongoTimestamp(6163845091342417920)
	c.Assert(mongo.NewMongoTimestamp(t), gc.Equals, expected)
	c.Assert(mongo.NewMongoTimestamp(t.In(time.UTC)), gc.Equals, expected)
}

func (s *oplogSuite) TestNewMongoTimestampBeforeUnixEpoch(c *gc.C) {
	c.Assert(mongo.NewMongoTimestamp(time.Time{}), gc.Equals, bson.MongoTimestamp(0))
}

func (s *oplogSuite) startMongoWithReplicaset(c *gc.C) (*jujutesting.MgoInstance, *mgo.Session) {
	inst := &jujutesting.MgoInstance{
		Params: []string{
			"--replSet", "juju",
		},
	}
	err := inst.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { inst.Destroy() })

	// Initiate replicaset.
	info := inst.DialInfo()
	args := peergrouper.InitiateMongoParams{
		DialInfo:       info,
		MemberHostPort: inst.Addr(),
	}
	err = peergrouper.MaybeInitiateMongoServer(args)
	c.Assert(err, jc.ErrorIsNil)

	return inst, s.dialMongo(c, inst)
}

func (s *oplogSuite) startMongo(c *gc.C) (*jujutesting.MgoInstance, *mgo.Session) {
	inst := &jujutesting.MgoInstance{
		Params: []string{
			"--setParameter", "enableTestCommands=1", // allows "emptycapped" command
		},
	}
	err := inst.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { inst.Destroy() })
	return inst, s.dialMongo(c, inst)
}

func (s *oplogSuite) emptyCapped(c *gc.C, coll *mgo.Collection) {
	// Call the emptycapped (test) command on a capped
	// collection. This invalidates any cursors on the collection.
	err := coll.Database.Run(bson.D{{"emptycapped", coll.Name}}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *oplogSuite) dialMongo(c *gc.C, inst *jujutesting.MgoInstance) *mgo.Session {
	session, err := inst.Dial()
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { session.Close() })
	return session
}

func (s *oplogSuite) makeFakeOplog(c *gc.C, session *mgo.Session) *mgo.Collection {
	db := session.DB("foo")
	oplog := db.C("oplog.fake")
	err := oplog.Create(&mgo.CollectionInfo{
		Capped:   true,
		MaxBytes: 1024 * 1024,
	})
	c.Assert(err, jc.ErrorIsNil)
	return oplog
}

func (s *oplogSuite) insertDoc(c *gc.C, srcSession *mgo.Session, coll *mgo.Collection, doc interface{}) {
	session := srcSession.Copy()
	defer session.Close()
	err := coll.With(session).Insert(doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *oplogSuite) getNextOplog(c *gc.C, tailer *mongo.OplogTailer) *mongo.OplogDoc {
	select {
	case doc, ok := <-tailer.Out():
		if !ok {
			c.Fatalf("tailer unexpectedly died: %v", tailer.Err())
		}
		return doc
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for oplog doc")
	}
	return nil
}

func (s *oplogSuite) assertNoOplog(c *gc.C, tailer *mongo.OplogTailer) {
	select {
	case _, ok := <-tailer.Out():
		if !ok {
			c.Fatalf("tailer unexpectedly died: %v", tailer.Err())
		}
		c.Fatal("unexpected oplog activity reported")
	case <-time.After(coretesting.ShortWait):
		// Success
	}
}

func (s *oplogSuite) assertStopped(c *gc.C, tailer *mongo.OplogTailer) {
	// Output should close.
	select {
	case _, ok := <-tailer.Out():
		c.Assert(ok, jc.IsFalse)
	case <-time.After(coretesting.LongWait):
		c.Fatal("tailer output should have closed")
	}

	// OplogTailer should die.
	select {
	case <-tailer.Dying():
		// Success.
	case <-time.After(coretesting.LongWait):
		c.Fatal("tailer should have died")
	}
}
