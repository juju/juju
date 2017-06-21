// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"fmt"
	"time"

	// 	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	// 	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	// 	"gopkg.in/tomb.v1"

	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/testing"
)

type PingBatcherSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	presence *mgo.Collection
	pings    *mgo.Collection
	modelTag names.ModelTag
}

var _ = gc.Suite(&PingBatcherSuite{})

func (s *PingBatcherSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.modelTag = names.NewModelTag(uuid.String())
}

func (s *PingBatcherSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *PingBatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)

	db := s.MgoSuite.Session.DB("presence")
	s.presence = db.C("presence")
	s.pings = db.C("presence.pings")

	presence.FakeTimeSlot(0)
}

func (s *PingBatcherSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)

	presence.RealTimeSlot()
	presence.RealPeriod()
}

func (s *PingBatcherSuite) TestRecordsPings(c *gc.C) {
	pb := presence.NewPingBatcher(s.presence, time.Second)
	defer assertStopped(c, pb)

	// UnixNano time rounded to 30s interval
	slot := int64(1497960150)
	c.Assert(pb.Ping("test-uuid", slot, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.Ping("test-uuid", slot, "0", 16), jc.ErrorIsNil)
	c.Assert(pb.Ping("test-uuid", slot, "1", 128), jc.ErrorIsNil)
	c.Assert(pb.Ping("test-uuid", slot, "1", 1), jc.ErrorIsNil)
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)
	docId := "test-uuid:1497960150"
	var res bson.M
	c.Assert(s.pings.FindId(docId).One(&res), jc.ErrorIsNil)
	c.Check(res["slot"], gc.Equals, slot)
	c.Check(res["alive"], jc.DeepEquals, bson.M{
		"0": int64(24),
		"1": int64(129),
	})
}

func (s *PingBatcherSuite) TestMultipleUUIDs(c *gc.C) {
	pb := presence.NewPingBatcher(s.presence, time.Second)
	defer assertStopped(c, pb)

	// UnixNano time rounded to 30s interval
	slot := int64(1497960150)
	uuid1 := "test-uuid1"
	uuid2 := "test-uuid2"
	c.Assert(pb.Ping(uuid1, slot, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.Ping(uuid2, slot, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.Ping(uuid2, slot, "0", 4), jc.ErrorIsNil)
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)
	docId1 := fmt.Sprintf("%s:%d", uuid1, slot)
	var res bson.M
	c.Assert(s.pings.FindId(docId1).One(&res), jc.ErrorIsNil)
	c.Check(res["slot"], gc.Equals, slot)
	c.Check(res["alive"], jc.DeepEquals, bson.M{
		"0": int64(8),
	})
	docId2 := fmt.Sprintf("%s:%d", uuid2, slot)
	c.Assert(s.pings.FindId(docId2).One(&res), jc.ErrorIsNil)
	c.Check(res["slot"], gc.Equals, slot)
	c.Check(res["alive"], jc.DeepEquals, bson.M{
		"0": int64(12),
	})
}

func (s *PingBatcherSuite) TestMultipleFlushes(c *gc.C) {
	pb := presence.NewPingBatcher(s.presence, time.Second)
	defer assertStopped(c, pb)

	slot := int64(1497960150)
	uuid1 := "test-uuid1"
	c.Assert(pb.Ping(uuid1, slot, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)

	docId1 := fmt.Sprintf("%s:%d", uuid1, slot)
	var res bson.M
	c.Assert(s.pings.FindId(docId1).One(&res), jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, bson.M{
		"_id":  docId1,
		"slot": slot,
		"alive": bson.M{
			"0": int64(8),
		},
	})

	c.Assert(pb.Ping(uuid1, slot, "0", 1024), jc.ErrorIsNil)
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)
	c.Assert(s.pings.FindId(docId1).One(&res), jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, bson.M{
		"_id":  docId1,
		"slot": slot,
		"alive": bson.M{
			"0": int64(1032),
		},
	})
}

func (s *PingBatcherSuite) TestMultipleSlots(c *gc.C) {
	pb := presence.NewPingBatcher(s.presence, time.Second)
	defer assertStopped(c, pb)

	slot1 := int64(1497960150)
	slot2 := int64(1497960180)
	uuid1 := "test-uuid1"
	c.Assert(pb.Ping(uuid1, slot1, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.Ping(uuid1, slot1, "0", 32), jc.ErrorIsNil)
	c.Assert(pb.Ping(uuid1, slot2, "1", 16), jc.ErrorIsNil)
	c.Assert(pb.Ping(uuid1, slot2, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)

	docId1 := fmt.Sprintf("%s:%d", uuid1, slot1)
	var res bson.M
	c.Assert(s.pings.FindId(docId1).One(&res), jc.ErrorIsNil)
	c.Check(res, gc.DeepEquals, bson.M{
		"_id":  docId1,
		"slot": slot1,
		"alive": bson.M{
			"0": int64(40),
		},
	})

	docId2 := fmt.Sprintf("%s:%d", uuid1, slot2)
	c.Assert(s.pings.FindId(docId2).One(&res), jc.ErrorIsNil)
	c.Check(res["slot"], gc.Equals, slot2)
	c.Check(res, gc.DeepEquals, bson.M{
		"_id":  docId2,
		"slot": slot2,
		"alive": bson.M{
			"0": int64(8),
			"1": int64(16),
		},
	})
}

func (s *PingBatcherSuite) TestDocBatchSize(c *gc.C) {
	// We don't want to hit an internal flush
	pb := presence.NewPingBatcher(s.presence, time.Hour)
	defer assertStopped(c, pb)

	slotBase := int64(1497960150)
	fieldKey := "0"
	fieldBit := uint64(64)
	// 100 slots * 100 models should be 10,000 docs that we are inserting.
	// mgo.Bulk fails if you try to do more than 1000 requests at once, so this would trigger it if we didn't batch properly.
	for modelCounter := 0; modelCounter < 100; modelCounter++ {
		for slotOffset := 0; slotOffset < 100; slotOffset++ {
			slot := slotBase + int64(slotOffset*30)
			uuid := fmt.Sprintf("uuid-%d", modelCounter)
			c.Assert(pb.Ping(uuid, slot, fieldKey, fieldBit), jc.ErrorIsNil)
		}
	}
	c.Assert(pb.ForceFlush(), jc.ErrorIsNil)
	count, err := s.pings.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 100*100)
}

func (s *PingBatcherSuite) TestBatchFlushesByTime(c *gc.C) {
	t := time.Now()
	pb := presence.NewPingBatcher(s.presence, 50*time.Millisecond)
	defer assertStopped(c, pb)

	slot := int64(1497960150)
	uuid := "test-uuid"
	c.Assert(pb.Ping("test-uuid", slot, "0", 8), jc.ErrorIsNil)
	c.Assert(pb.Ping("test-uuid", slot, "0", 16), jc.ErrorIsNil)
	docId := fmt.Sprintf("%s:%d", uuid, slot)
	var res bson.M
	// We should not have found it yet
	c.Assert(s.pings.FindId(docId).One(&res), gc.Equals, mgo.ErrNotFound)
	// We will wait up to 1s for the write to succeed. While waiting, we will ping on another slot, to
	// hint at other pingers that are still active, without messing up our final assertion.
	slot2 := slot + 30
	for i := 0; i < 1000; i++ {
		time.Sleep(time.Millisecond)
		err := s.pings.FindId(docId).One(&res)
		slot2 = slot2 + 30
		pb.Ping("test-uuid", slot2, "0", 1)
		waitTime := time.Since(t)
		if waitTime < time.Duration(35*time.Millisecond) {
			// Officially it should take a minimum of 50*0.8 = 40ms.
			// make sure the timer hasn't flushed yet
			c.Assert(err, gc.Equals, mgo.ErrNotFound,
				gc.Commentf("PingBatcher flushed too soon, expected at least 15ms"))
			continue
		}
		if err == nil {
			c.Logf("found the document after %v", waitTime)
			break
		} else {
			c.Logf("no document after %v", waitTime)
			c.Assert(err, gc.Equals, mgo.ErrNotFound)
		}
	}
	// If it wasn't found, this check will fail
	c.Check(res, gc.DeepEquals, bson.M{
		"_id":  docId,
		"slot": slot,
		"alive": bson.M{
			"0": int64(24),
		},
	})
}

func (s *PingBatcherSuite) TestStoppedPingerRejectsPings(c *gc.C) {
	pb := presence.NewPingBatcher(s.presence, testing.ShortWait)
	defer assertStopped(c, pb)
	c.Assert(pb.Stop(), jc.ErrorIsNil)
	slot := int64(1497960150)
	uuid := "test-uuid"
	err := pb.Ping(uuid, slot, "0", 8)
	c.Assert(err, gc.ErrorMatches, "PingBatcher is stopped")
}

func (s *PingBatcherSuite) TestNewDeadPingBatcher(c *gc.C) {
	testErr := fmt.Errorf("this is an error")
	pb := presence.NewDeadPingBatcher(testErr)
	slot := int64(1497960150)
	uuid := "test-uuid"
	err := pb.Ping(uuid, slot, "0", 8)
	c.Assert(err, gc.ErrorMatches, "this is an error")

	err = pb.Stop()
	c.Assert(err, gc.ErrorMatches, "this is an error")
}
