// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package presence

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/tomb.v1"
)

const (
	maxBatch         = 1000
	defaultSyncDelay = 10 * time.Millisecond
)

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
		syncChan:      make(chan chan struct{}),
		syncDelay:     defaultSyncDelay,
		rand:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	pb.start()
	return pb
}

// NewDeadPingBatcher returns a PingBatcher that is already stopped with an error.
func NewDeadPingBatcher(err error) *PingBatcher {
	// we never start the loop, so the timeout doesn't matter.
	pb := &PingBatcher{}
	pb.tomb.Kill(err)
	pb.tomb.Done()
	return pb
}

// PingBatcher aggregates several pingers to update the database on a fixed schedule.
type PingBatcher struct {

	// pings is the collection where we record our information
	pings *mgo.Collection

	// pending is the list of pings that have not been written to the database yet
	pending map[string]slot

	// pingCount is how many pings we've received that we have not flushed
	pingCount uint64

	// flushInterval is the nominal amount of time where we will automatically flush
	flushInterval time.Duration

	// rand is a random source used to vary our nominal flushInterval
	rand *rand.Rand

	// tomb is used to track a request to shutdown this worker
	tomb tomb.Tomb

	// pingChan is where requests from Ping() are brought into the main loop
	pingChan chan singlePing

	// syncChan is where explicit requests to flush come in
	syncChan chan chan struct{}

	// syncDelay is the time we will wait before triggering a flush after a
	// sync request comes in. We don't do it immediately so that many agents
	// waking all issuing their initial request still don't flood the database
	// with separate requests, but we do respond faster than normal.
	syncDelay time.Duration

	// awaitingSync is the slice of requests that are waiting for flush to finish
	awaitingSync []chan struct{}

	// flushMutex ensures only one concurrent flush is done
	flushMutex sync.Mutex
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
func (pb *PingBatcher) nextSleep(r *rand.Rand) time.Duration {
	sleepMin := float64(pb.flushInterval) * 0.8
	sleepRange := float64(pb.flushInterval) * 0.4
	offset := r.Int63n(int64(sleepRange))
	return time.Duration(int64(sleepMin) + offset)
}

func (pb *PingBatcher) loop() error {
	flushTimeout := time.After(pb.nextSleep(pb.rand))
	var syncTimeout <-chan time.Time
	for {
		doflush := func() error {
			syncTimeout = nil
			err := pb.flush()
			flushTimeout = time.After(pb.nextSleep(pb.rand))
			return errors.Trace(err)
		}
		select {
		case <-pb.tomb.Dying():
			// We were asked to shut down. Make sure we flush
			if err := pb.flush(); err != nil {
				return errors.Trace(err)
			}
			return errors.Trace(tomb.ErrDying)
		case singlePing := <-pb.pingChan:
			pb.handlePing(singlePing)
		case syncReq := <-pb.syncChan:
			// Flush is requested synchronously.
			// The caller passes in a channel we can close so that
			// they know when we have finished flushing.
			// We also know that any "Ping()" requests that have
			// returned will have been handled before Flush()
			// because they are all serialized in this loop.
			// We need to guard access to pb.awaitingSync as tests
			// poke this asynchronously.
			pb.flushMutex.Lock()
			pb.awaitingSync = append(pb.awaitingSync, syncReq)
			pb.flushMutex.Unlock()
			if syncTimeout == nil {
				syncTimeout = time.After(pb.syncDelay)
			}
		case <-syncTimeout:
			// Golang says I can't use 'fallthrough' here, but I
			// want to do exactly the same thing if either of the channels trigger
			// fallthrough
			if err := doflush(); err != nil {
				return errors.Trace(err)
			}
		case <-flushTimeout:
			if err := doflush(); err != nil {
				return errors.Trace(err)
			}
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

// Sync schedules a flush of the current state to the database.
// This is not immediate, but actually within a short timeout so that many calls
// to sync in a short time frame will only trigger one write to the database.
func (pb *PingBatcher) Sync() error {
	request := make(chan struct{})
	select {
	case pb.syncChan <- request:
		select {
		case <-request:
			return nil
		case <-pb.tomb.Dying():
			break
		}
	case <-pb.tomb.Dying():
		break
	}
	if err := pb.tomb.Err(); err == nil {
		return errors.Errorf("PingBatcher is stopped")
	} else {
		return err
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
	pb.flushMutex.Lock()
	defer pb.flushMutex.Unlock()

	awaiting := pb.awaitingSync
	pb.awaitingSync = nil
	// We are doing a flush, make sure everyone waiting is told that it has been done
	defer func() {
		for _, waiting := range awaiting {
			close(waiting)
		}
	}()
	if pb.pingCount == 0 {
		return nil
	}
	uuids := set.NewStrings()
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
		if logger.IsTraceEnabled() {
			// the rest of Pings records the first 6 characters of
			// model-uuids, so we include that here if we are TRACEing.
			uuids.Add(docId[:6])
		}
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
	logger.Tracef("[%v] pingbatcher recorded %d pings for %d ping slot(s) and %d fields in %v",
		strings.Join(uuids.SortedValues(), ", "), pingCount, docCount, fieldCount, time.Since(t))
	return nil
}
