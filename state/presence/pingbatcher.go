// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package presence

import (
	"math/rand"
	"sync"
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

// NewPingBatcher creates a worker that will batch ping requests and prepare them
// for insertion into the Pings collection. Pass in the base "presence" collection.
// flushInterval is how often we will write the contents to the database.
// It should be shorter than the 30s slot window for us to not cause active
// pingers to show up as missing. Current defaults are around 1s, but testing needs
// to be done to find a good balance of how much batching we do vs responsiveness.
// 10s might actually be a reasonable value, given the 30s windows.
// Note that we don't strictly sync on flushInterval times, but use a range of
// times around that interval to avoid having all ping batchers get synchronized
// and still be issuing all requests concurrently.
func NewPingBatcher(base *mgo.Collection, flushInterval time.Duration) *PingBatcher {
	var pings *mgo.Collection
	if base != nil {
		pings = pingsC(base)
	}
	return &PingBatcher{
		pings:         pings,
		pending:       make(map[string]slot),
		flushInterval: flushInterval,
		flushChan:     make(chan chan struct{}),
	}
}

// NewDeadPingBatcher returns a PingBatcher that is already stopped with an error.
func NewDeadPingBatcher(err error) *PingBatcher {
	// we never start the loop, so the timeout doesn't matter.
	pb := NewPingBatcher(nil, time.Millisecond)
	pb.tomb.Kill(err)
	pb.tomb.Done()
	return pb
}

// PingBatcher aggregates several pingers to update the database on a fixed schedule.
type PingBatcher struct {
	pings         *mgo.Collection
	modelUUID     string
	mu            sync.Mutex
	pending       map[string]slot
	pingCount     uint64
	flushInterval time.Duration
	tomb          tomb.Tomb
	started       bool
	flushChan     chan chan struct{}
}

// Start the worker loop.
func (pb *PingBatcher) Start() error {
	rand.Seed(time.Now().UnixNano())
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
	pb.mu.Lock()
	pb.started = true
	pb.mu.Unlock()
	return nil
}

// Kill is part of the worker.Worker interface.
func (pb *PingBatcher) Kill() {
	logger.Debugf("PingBatcher.Kill() called")
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
	offset := rand.Int63n(int64(sleepRange))
	return time.Duration(int64(sleepMin) + offset)
}

func (pb *PingBatcher) loop() error {
	// flushDone and flushRequest exist to make a flushRequest synchronous
	// with all other pings that have been requested. The logic is:
	for {
		select {
		case <-pb.tomb.Dying():
			logger.Debugf("PingBatcher Dying")
			pb.mu.Lock()
			pb.started = false
			pb.mu.Unlock()
			return errors.Trace(tomb.ErrDying)
		case <-time.After(pb.nextSleep()):
			if err := pb.flush(); err != nil {
				return errors.Trace(err)
			}
		case flushReq := <-pb.flushChan:
			// Flush is requested synchronously.
			// This way we know all pings have been handled. We will
			// close the channel that was passed to us once we have
			// finished flushing.
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
func (pb *PingBatcher) Ping(modelUUID string, slot int64, fieldKey string, fieldBit uint64) error {
	// We use a 'direct-update' model rather than synchronizing with the main loop.
	// In testing '10,000' concurrent pingers, trying to synchronize with the main loop
	// actually caused starvation where the timeout triggered very infrequently.
	// (over 30s with a 25ms timeout, there were only 5 flush requests).
	// Using a mutex and doing the internal-state update had the same total time spent
	// doing updates, but meant that we could reliably trigger flushing to the db.
	// My guess is that channels are "fair" to the people sending, and 10,000 things
	// trying to send on one channel vs 1 thing sending on another channel.
	// What we wanted was fairness between channels vs fairness between senders.
	pb.mu.Lock()
	if !pb.started {
		// Should this be more pb.Tomb.Err() ?
		pb.mu.Unlock()
		return errors.Errorf("PingBatcher not started")
	}
	docId := docIDInt64(modelUUID, slot)
	cur, slotExists := pb.pending[docId]
	if !slotExists {
		cur.Alive = make(map[string]uint64)
		cur.Slot = slot
		pb.pending[docId] = cur
	}
	alive := cur.Alive
	alive[fieldKey] |= fieldBit
	pb.pingCount++
	pb.mu.Unlock()
	return nil
}

// flush pushes the internal state to the database. Note that if the database
// updates fail, we will still wipe our internal state as it is unsafe to
// publish the same updates to the same slots.
// TODO (jam): 2017-06-21 Maybe if we switched from "$inc" to "$bit":
// https://docs.mongodb.com/manual/reference/operator/update/bit/
// Bitwise OR was supported all the way back to Mongo 2.2
func (pb *PingBatcher) flush() error {
	// We treat all of these as 'consumed'. Even if the query fails, it is
	// not safe to ever $inc the same fields a second time, so we just move on.
	// We grab a mutex just long enough to create a copy of the internal structures,
	// and reset them to their empty values. New Pings() can be handled concurrently while
	// we write the current set to the database.
	pb.mu.Lock()
	toFlush := pb.pending
	pingCount := pb.pingCount
	pb.pending = make(map[string]slot)
	pb.pingCount = 0
	session := pb.pings.Database.Session.Copy()
	defer session.Close()
	pings := pb.pings.With(session)
	bulk := pings.Bulk()
	pb.mu.Unlock()
	docCount := 0
	fieldCount := 0
	t := time.Now()
	bulkCount := 0
	for docId, slot := range toFlush {
		docCount++
		var incFields bson.D
		for fieldKey, value := range slot.Alive {
			incFields = append(incFields, bson.DocElem{Name: "alive." + fieldKey, Value: value})
			fieldCount++
		}
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
