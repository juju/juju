// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package presence

import (
	"math/rand"
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v1"
)

const maxBatch = 1000

type slot struct {
	Slot  int64
	Alive map[string]uint64
}

type singlePing struct {
	Slot      int64
	ModelUUID string
	FieldKey  string
	FieldBit  uint64
}

// NewPingBatcher creates a worker that will batch ping requests and prepare them
// for insertion into the Pings collection. Pass in the base "presence" collection.
// flushInterval is how often we will write the contents to the database.
// It should be shorter than the 30s slot window for us to not cause active
// pingers to show up as missing. The current default is 1s as it provides a good
// balance of significant-batching-for-performance while still having responsiveness
// to agents coming alive.
// Note that we don't strictly sync on flushInterval times, but use a range of
// times around that interval to avoid having all ping batchers get synchronized
// and still be issuing all requests concurrently.
func NewPingBatcher(base *mgo.Collection, flushInterval time.Duration) *PingBatcher {
	var pings *mgo.Collection
	if base != nil {
		pings = pingsC(base)
	}
	pb := &PingBatcher{
		pings:         pings,
		pending:       make(map[string]slot),
		flushInterval: flushInterval,
		pingChan:      make(chan singlePing),
		flushChan:     make(chan chan struct{}),
		rand:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	pb.start()
	return pb
}

// NewDeadPingBatcher returns a PingBatcher that is already stopped with an error.
func NewDeadPingBatcher(err error) *PingBatcher {
	// we never start the loop, so the timeout doesn't matter.
	pb := &PingBatcher{
		pings:         nil,
		pending:       make(map[string]slot),
		flushInterval: time.Millisecond,
		flushChan:     make(chan chan struct{}),
	}
	pb.tomb.Kill(err)
	pb.tomb.Done()
	return pb
}

// PingBatcher aggregates several pingers to update the database on a fixed schedule.
type PingBatcher struct {
	pings         *mgo.Collection
	modelUUID     string
	pending       map[string]slot
	pingCount     uint64
	flushInterval time.Duration
	tomb          tomb.Tomb
	pingChan      chan singlePing
	flushChan     chan chan struct{}
	rand          *rand.Rand
}

// Start the worker loop.
func (pb *PingBatcher) start() {
	go func() {
		err := pb.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("ping batching loop failed: %v", err)
		}
		pb.tomb.Kill(cause)
		pb.tomb.Done()
	}()
}

// Kill is part of the worker.Worker interface.
func (pb *PingBatcher) Kill() {
	pb.tomb.Kill(nil)
}

// Wait returns when the Pinger has stopped, and returns the first error
// it encountered.
func (pb *PingBatcher) Wait() error {
	return pb.tomb.Wait()
}

// Stop this PingBatcher, part of the extended Worker interface.
func (pb *PingBatcher) Stop() error {
	pb.tomb.Kill(nil)
	err := pb.tomb.Wait()
	return errors.Trace(err)
}

// nextSleep determines how long we should wait before flushing our state to the database.
// We use a range of time around the requested 'flushInterval', so that we avoid having
// all requests to the database happen at exactly the same time across machines.
func (pb *PingBatcher) nextSleep() time.Duration {
	sleepMin := float64(pb.flushInterval) * 0.8
	sleepRange := float64(pb.flushInterval) * 0.4
	offset := pb.rand.Int63n(int64(sleepRange))
	return time.Duration(int64(sleepMin) + offset)
}

func (pb *PingBatcher) loop() error {
	flushTimeout := time.After(pb.nextSleep())
	for {
		select {
		case <-pb.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-flushTimeout:
			if err := pb.flush(); err != nil {
				return errors.Trace(err)
			}
			flushTimeout = time.After(pb.nextSleep())
		case singlePing := <-pb.pingChan:
			pb.handlePing(singlePing)
		case flushReq := <-pb.flushChan:
			// Flush is requested synchronously.
			// The caller passes in a channel we can close so that
			// they know when we have finished flushing.
			// We also know that any "Ping()" requests that have
			// returned will have been handled before Flush()
			// because they are all serialized in this loop.
			if err := pb.flush(); err != nil {
				return errors.Trace(err)
			}
			close(flushReq)
		}
	}
}

// Ping should be called by a Pinger when it is ready to update its time slot.
// It passes in all of the pre-resolved information (what exact field bit is
// being set), rather than the higher level "I'm pinging for this Agent".
// Internally, we synchronize with the main worker loop. Which means that this
// function will return once the main loop recognizes that we have a ping request
// but it will not have updated its internal structures, and certainly not the database.
func (pb *PingBatcher) Ping(modelUUID string, slot int64, fieldKey string, fieldBit uint64) error {
	ping := singlePing{
		Slot:      slot,
		ModelUUID: modelUUID,
		FieldKey:  fieldKey,
		FieldBit:  fieldBit,
	}
	select {
	case pb.pingChan <- ping:
		return nil
	case <-pb.tomb.Dying():
		err := pb.tomb.Err()
		if err == nil {
			return errors.Errorf("PingBatcher is stopped")
		}
		return errors.Trace(err)
	}
}

// Sync immediately flushes the current state to the database.
// This should generally only be called from testing code, everyone else can
// generally wait the usual wait for updates to be flushed naturally.
func (pb *PingBatcher) Sync() error {
	request := make(chan struct{})
	select {
	case pb.flushChan <- request:
		select {
		case <-request:
			return nil
		case <-pb.tomb.Dying():
			return pb.tomb.Err()
		}
	case <-pb.tomb.Dying():
		return pb.tomb.Err()
	}
}

// handlePing is where we actually update our internal structures after we
// get a ping request.
func (pb *PingBatcher) handlePing(ping singlePing) {
	docId := docIDInt64(ping.ModelUUID, ping.Slot)
	cur, slotExists := pb.pending[docId]
	if !slotExists {
		cur.Alive = make(map[string]uint64)
		cur.Slot = ping.Slot
		pb.pending[docId] = cur
	}
	alive := cur.Alive
	alive[ping.FieldKey] |= ping.FieldBit
	pb.pingCount++
}

// flush pushes the internal state to the database. Note that if the database
// updates fail, we will still wipe our internal state as it is unsafe to
// publish the same updates to the same slots.
func (pb *PingBatcher) flush() error {
	// We treat all of these as 'consumed'. Even if the query fails, it is
	// not safe to ever $inc the same fields a second time, so we just move on.
	next := pb.pending
	pingCount := pb.pingCount
	pb.pending = make(map[string]slot)
	pb.pingCount = 0
	session := pb.pings.Database.Session.Copy()
	defer session.Close()
	pings := pb.pings.With(session)
	bulk := pings.Bulk()
	docCount := 0
	fieldCount := 0
	t := time.Now()
	bulkCount := 0
	for docId, slot := range next {
		docCount++
		var incFields bson.D
		for fieldKey, value := range slot.Alive {
			incFields = append(incFields, bson.DocElem{Name: "alive." + fieldKey, Value: value})
			fieldCount++
		}
		// TODO(jam): 2016-06-22 https://bugs.launchpad.net/juju/+bug/1699678
		// Consider switching $inc to $bit {or }. It would let us cleanup
		// presence.beings a lot if we didn't have to worry about pinging
		// the same slot twice.
		bulk.Upsert(
			bson.D{{"_id", docId}},
			bson.D{
				{"$set", bson.D{{"slot", slot.Slot}}},
				{"$inc", incFields},
			},
		)
		bulkCount++
		if bulkCount >= maxBatch {
			if _, err := bulk.Run(); err != nil {
				return errors.Trace(err)
			}
			bulkCount = 0
			bulk = pings.Bulk()
		}
	}
	if bulkCount > 0 {
		if _, err := bulk.Run(); err != nil {
			return errors.Trace(err)
		}
	}
	// usually we should only be processing 1 slot
	logger.Debugf("recorded %d pings for %d ping slot(s) and %d fields in %v",
		pingCount, docCount, fieldCount, time.Since(t))
	return nil
}
