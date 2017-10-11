// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func FakeTimeSlot(offset int) {
	fakeTimeSlot(offset)
}

func RealTimeSlot() {
	realTimeSlot()
}

func FakePeriod(seconds int64) {
	period = seconds
}

var realPeriod = period

func RealPeriod() {
	period = realPeriod
}

func DirectRecordFunc(base *mgo.Collection) PingRecorder {
	return &directRecorder{pings: pingsC(base)}
}

type directRecorder struct {
	pings *mgo.Collection
}

func (dr *directRecorder) Ping(modelUUID string, slot int64, fieldKey string, fieldBit uint64) error {
	session := dr.pings.Database.Session.Copy()
	defer session.Close()
	pings := dr.pings.With(session)
	_, err := pings.UpsertId(
		docIDInt64(modelUUID, slot),
		bson.D{
			{"$set", bson.D{{"slot", slot}}},
			{"$inc", bson.D{{"alive." + fieldKey, fieldBit}}},
		})
	return err
}

// ForceInc forces the PingBatcher to use $inc instead of $bit operations
// This exists to test the code path that runs when using Mongo 2.4 and older.
func (pb *PingBatcher) ForceUpdatesUsingInc() {
	logger.Debugf("forcing $inc operations from (was %t)", pb.useInc)
	pb.useInc = true
}
